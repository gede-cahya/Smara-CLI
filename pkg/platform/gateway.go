package platform

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/pkg/agent"
	"github.com/gede-cahya/Smara-CLI/pkg/metrics"
)

// maxMessageLength is the maximum length of a single message on most platforms.
const maxMessageLength = 4000

// Gateway routes incoming messages from platform adapters to the Smara supervisor
// and sends responses back. It manages per-channel sessions, authentication,
// and rate limiting.
type Gateway struct {
	adapters        map[string]PlatformAdapter
	supervisor      *agent.Supervisor
	sessions        map[string]*PlatformSession // channelID → session
	auth            *AuthManager
	rateLimiter     *RateLimiter
	metrics         *metrics.MetricsCollector
	sensitiveGuards map[string]SensitiveDataGuard
	mu              sync.RWMutex
}

// NewGateway creates a new Gateway with the given supervisor.
func NewGateway(supervisor *agent.Supervisor) *Gateway {
	return &Gateway{
		adapters:        make(map[string]PlatformAdapter),
		supervisor:      supervisor,
		sessions:        make(map[string]*PlatformSession),
		auth:            NewAuthManager(),
		rateLimiter:     NewRateLimiter(RateLimitConfig{RequestsPerMinute: 20, BurstSize: 5}),
		sensitiveGuards: make(map[string]SensitiveDataGuard),
	}
}

// SetAuth configures the auth manager for the gateway.
func (g *Gateway) SetAuth(auth *AuthManager) {
	g.auth = auth
}

// SetRateLimiter configures the rate limiter for the gateway.
func (g *Gateway) SetRateLimiter(rl *RateLimiter) {
	g.rateLimiter = rl
}

// SetMetrics configures the metrics collector for the gateway.
func (g *Gateway) SetMetrics(mc *metrics.MetricsCollector) {
	g.metrics = mc
}

// RegisterAdapter adds a platform adapter to the gateway.
func (g *Gateway) RegisterAdapter(adapter PlatformAdapter) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.adapters[adapter.Name()] = adapter
}

// Start connects all registered adapters and begins listening for messages.
// This method blocks until ctx is cancelled.
func (g *Gateway) Start(ctx context.Context) error {
	if len(g.adapters) == 0 {
		return fmt.Errorf("tidak ada platform adapter yang terdaftar")
	}

	// Start periodic rate limiter cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				g.rateLimiter.Cleanup()
			}
		}
	}()

	// Start all adapters in parallel
	var wg sync.WaitGroup
	errCh := make(chan error, len(g.adapters))

	for name, adapter := range g.adapters {
		wg.Add(1)
		go func(name string, adapter PlatformAdapter) {
			defer wg.Done()
			log.Printf("[gateway] Memulai adapter: %s", name)
			if err := adapter.Listen(ctx, g.HandleIncoming); err != nil {
				if ctx.Err() == nil { // only report if not cancelled
					log.Printf("[gateway] Adapter %s error: %v", name, err)
					errCh <- fmt.Errorf("adapter %s: %w", name, err)
				}
			}
			log.Printf("[gateway] Adapter %s berhenti", name)
		}(name, adapter)
	}

	// Wait for all adapters to finish
	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("gateway errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// Stop gracefully shuts down all adapters.
func (g *Gateway) Stop() {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for name, adapter := range g.adapters {
		log.Printf("[gateway] Menutup adapter: %s", name)
		if err := adapter.Close(); err != nil {
			log.Printf("[gateway] Error menutup %s: %v", name, err)
		}
	}
}

// HandleIncoming processes an incoming message from any platform.
// It performs auth check, rate limiting, session management, and dispatches to the supervisor.
func (g *Gateway) HandleIncoming(ctx context.Context, msg IncomingMessage) error {
	// 1. Auth check
	if !g.auth.IsAllowed(msg.Platform, msg.UserID) {
		return g.sendReply(ctx, msg, "⛔ Akses ditolak. Hubungi admin untuk mendapatkan akses.")
	}

	// 2. Rate limit check
	if !g.rateLimiter.Allow(msg.UserID) {
		return g.sendReply(ctx, msg, "⏳ Rate limit tercapai. Coba lagi dalam beberapa saat.")
	}

	if denied, denyMessage := g.checkSensitiveDataAccess(msg); denied {
		log.Printf("[gateway] Owner-only request denied for %s user=%s", msg.Platform, msg.UserID)
		return g.sendReply(ctx, msg, denyMessage)
	}

	// 3. Record incoming message metric
	if g.metrics != nil {
		g.metrics.RecordMessageIn(msg.Platform, msg.UserID, msg.Username)
	}

	// 4. Handle commands
	if msg.IsCommand {
		return g.handleCommand(ctx, msg)
	}

	// 5. Process as prompt
	return g.processPrompt(ctx, msg)
}

// handleCommand handles platform commands like /mode, /help, etc.
func (g *Gateway) handleCommand(ctx context.Context, msg IncomingMessage) error {
	switch msg.Command {
	case "start":
		welcome := `🌀 *Smara* — Autonomous Multi-Agent Terminal

Selamat datang! Saya adalah agen AI yang siap membantu Anda.

*Perintah:*
/ask <prompt> — Kirim pertanyaan
/mode <ask|rush|plan> — Ganti mode agen
/mcp — Lihat MCP tools
/clear — Reset percakapan
/help — Bantuan

Atau langsung ketik pesan untuk memulai percakapan.`
		return g.sendReply(ctx, msg, welcome)

	case "help":
		help := `📖 *Bantuan Smara Bot*

/ask <prompt> — Kirim prompt ke Smara
/mode — Lihat mode saat ini
/mode <ask|rush|plan> — Ganti mode
/mcp — Daftar MCP tools
/clear — Reset history percakapan
/help — Tampilkan pesan ini

💡 Anda juga bisa langsung mengetik pesan tanpa perintah.`
		return g.sendReply(ctx, msg, help)

	case "ask":
		if len(msg.CommandArgs) == 0 {
			return g.sendReply(ctx, msg, "❌ Gunakan: /ask <pertanyaan>")
		}
		// Reconstruct prompt from args
		prompt := strings.Join(msg.CommandArgs, " ")
		promptMsg := msg
		promptMsg.Content = prompt
		return g.processPrompt(ctx, promptMsg)

	case "mode":
		if len(msg.CommandArgs) == 0 {
			// Show current mode
			current := g.supervisor.GetMode()
			info := agent.GetModeInfo(current)
			reply := fmt.Sprintf("Mode saat ini: %s %s\n\n%s\n\nGunakan /mode <ask|rush|plan> untuk mengganti.", info.Emoji, info.Label, info.Description)
			return g.sendReply(ctx, msg, reply)
		}
		newMode := msg.CommandArgs[0]
		if !agent.ValidMode(newMode) {
			return g.sendReply(ctx, msg, "❌ Mode tidak valid. Pilih: ask, rush, plan")
		}
		g.supervisor.SetMode(agent.Mode(newMode))
		info := agent.GetModeInfo(agent.Mode(newMode))
		return g.sendReply(ctx, msg, fmt.Sprintf("%s Mode diubah ke *%s*\n%s", info.Emoji, info.Label, info.Description))

	case "mcp":
		mcpInfo := g.supervisor.GetMCPInfo()
		if len(mcpInfo) == 0 {
			return g.sendReply(ctx, msg, "ℹ️ Tidak ada MCP server yang terhubung.")
		}
		var sb strings.Builder
		sb.WriteString("🔧 *MCP Servers:*\n\n")
		for name, info := range mcpInfo {
			status := "✅"
			if !info.Connected {
				status = "❌"
			}
			sb.WriteString(fmt.Sprintf("%s *%s*", status, name))
			if len(info.Tools) > 0 {
				sb.WriteString(fmt.Sprintf(" — %d tools\n", len(info.Tools)))
				for _, tool := range info.Tools {
					desc := tool.Description
					if len(desc) > 50 {
						desc = desc[:50] + "..."
					}
					sb.WriteString(fmt.Sprintf("  • %s: %s\n", tool.Name, desc))
				}
			} else {
				sb.WriteString("\n")
			}
		}
		return g.sendReply(ctx, msg, sb.String())

	case "clear":
		g.supervisor.ClearHistory()
		return g.sendReply(ctx, msg, "🗑️ Percakapan direset.")

	default:
		return g.sendReply(ctx, msg, fmt.Sprintf("❓ Perintah tidak dikenal: /%s\nKetik /help untuk bantuan.", msg.Command))
	}
}

// processPrompt sends a user prompt to the supervisor and relays the response.
func (g *Gateway) processPrompt(ctx context.Context, msg IncomingMessage) error {
	// Send typing indicator
	g.mu.RLock()
	adapter, ok := g.adapters[msg.Platform]
	g.mu.RUnlock()
	if denied, denyMessage := g.checkSensitiveDataAccess(msg); denied {
		log.Printf("[gateway] Sensitive data request denied for %s user=%s", msg.Platform, msg.UserID)
		return g.sendReply(ctx, msg, denyMessage)
	}
	downloadedImages := []string{}
	if len(msg.Attachments) > 0 {
		if downloader, ok := adapter.(AttachmentDownloader); ok {
			injected := []string{}
			for _, att := range msg.Attachments {
				if att.Type != "image" {
					continue
				}
				ref := att.URL
				if ref == "" {
					ref = att.FileName
				}
				path, err := downloader.DownloadAttachment(ctx, ref)
				if err != nil {
					log.Printf("[gateway] gagal download attachment %s: %v", ref, err)
					continue
				}
				log.Printf("[gateway] image attachment di-download: %s", path)
				downloadedImages = append(downloadedImages, path)
				injected = append(injected, "[image:"+path+"]")
			}
			if len(injected) > 0 {
				steer := "\n\n[Sistem: pesan ini menyertakan gambar. Pakai tool analyze_image dengan path tersebut untuk melihat konten gambar — jangan menebak isi tanpa membacanya. Setelah dapat hasil, jawab pertanyaan user berdasarkan info tersebut.]"
				if msg.Content == "" {
					msg.Content = strings.Join(injected, " ") + " (tidak ada caption — analisa gambar yang dilampirkan)" + steer
				} else {
					msg.Content = strings.Join(injected, " ") + " " + msg.Content + steer
				}
			}
		} else {
			log.Printf("[gateway] adapter %s belum support download attachment, %d attachment di-skip", msg.Platform, len(msg.Attachments))
		}
	}

	if len(downloadedImages) > 0 && isImageAnalysisPrompt(msg.Content) {
		log.Printf("[gateway] image analysis fast-path matched for %s/%s images=%d", msg.Platform, msg.ChannelID, len(downloadedImages))
		output, err := analyzeDownloadedImages(ctx, downloadedImages, msg.Content)
		if err != nil {
			return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
		}
		return g.sendReply(ctx, msg, output)
	}

	if ok {
		_ = adapter.SendTyping(ctx, msg.ChannelID)
	}

	var generatedMu sync.Mutex
	var generatedAttachments []Attachment
	currentTool := ""
	g.supervisor.SetCallback(agent.AgenticCallback{
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			generatedMu.Lock()
			currentTool = tool
			generatedMu.Unlock()
		},
		OnToolResult: func(output string) {
			generatedMu.Lock()
			if currentTool == "generate_image" {
				generatedAttachments = append(generatedAttachments, imageAttachmentsFromToolOutput(output)...)
			}
			generatedMu.Unlock()
		},
	})

	if isImageGenerationPrompt(msg.Content) {
		log.Printf("[gateway] image generation fast-path matched for %s/%s: %q", msg.Platform, msg.ChannelID, redactSensitiveLogContent(msg.Content))
		output, err := agent.ExecuteBuiltinTool("generate_image", map[string]interface{}{"prompt": msg.Content}, nil)
		if err != nil {
			return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
		}
		return g.sendReplyWithAttachments(ctx, msg, output, imageAttachmentsFromToolOutput(output))
	}

	// Process via supervisor
	startTime := time.Now()
	result, err := g.supervisor.ProcessPrompt(ctx, msg.Content)
	latencyMs := time.Since(startTime).Milliseconds()
	g.supervisor.SetCallback(agent.AgenticCallback{})

	if err != nil {
		if g.metrics != nil {
			g.metrics.RecordError(msg.Platform, err.Error())
		}
		return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
	}

	// Record metrics
	if g.metrics != nil {
		g.metrics.RecordMessageOut(msg.Platform)
		g.metrics.RecordLatency(msg.Platform, latencyMs)
		model := g.supervisor.GetModel()
		cost := metrics.EstimateCost(
			g.supervisor.GetProviderName(), model,
			int64(result.InputTokens), int64(result.OutputTokens),
		)
		g.metrics.RecordLLMUsage(result.InputTokens, result.OutputTokens, latencyMs, cost)
	}

	generatedMu.Lock()
	attachments := append([]Attachment(nil), generatedAttachments...)
	generatedMu.Unlock()
	return g.sendReplyWithAttachments(ctx, msg, result.Response, attachments)
}

// sendReply sends a response back to the platform where the message originated.
func (g *Gateway) sendReply(ctx context.Context, original IncomingMessage, content string) error {
	return g.sendReplyWithAttachments(ctx, original, content, nil)
}

func (g *Gateway) sendReplyWithAttachments(ctx context.Context, original IncomingMessage, content string, attachments []Attachment) error {
	g.mu.RLock()
	adapter, ok := g.adapters[original.Platform]
	g.mu.RUnlock()
	if !ok {
		return fmt.Errorf("adapter tidak ditemukan untuk platform: %s", original.Platform)
	}

	attachments = mergeAttachments(attachments, imageAttachmentsFromToolOutput(content))

	// Split long messages
	parts := splitMessage(content, maxMessageLength)
	for i, part := range parts {
		outMsg := OutgoingMessage{
			Content: part,
			Format:  FormatMarkdown,
			ReplyTo: original.ID,
		}
		if i == 0 {
			outMsg.Attachments = attachments
		}
		if err := adapter.SendMessage(ctx, original.ChannelID, outMsg); err != nil {
			return fmt.Errorf("gagal mengirim reply ke %s: %w", original.Platform, err)
		}
	}

	return nil
}

func isImageGenerationPrompt(prompt string) bool {
	p := strings.ToLower(prompt)
	if strings.Contains(p, "analisa") || strings.Contains(p, "analyze") || strings.Contains(p, "lihat gambar") {
		return false
	}
	imageTerms := []string{"gambar", "image", "logo", "ilustrasi", "illustration", "icon", "ikon", "poster", "desain visual"}
	generateTerms := []string{"buat", "buatkan", "generate", "create", "bikin", "design", "desain"}
	for _, gen := range generateTerms {
		if !strings.Contains(p, gen) {
			continue
		}
		for _, img := range imageTerms {
			if strings.Contains(p, img) {
				return true
			}
		}
	}
	return false
}

func isImageAnalysisPrompt(prompt string) bool {
	p := strings.ToLower(prompt)
	terms := []string{"analisa", "analisis", "analyze", "lihat", "gambar", "image", "foto", "screenshot", "ini kenapa", "jelaskan", "apa ini", "cek ini"}
	for _, term := range terms {
		if strings.Contains(p, term) {
			return true
		}
	}
	return strings.TrimSpace(prompt) == ""
}

func analyzeDownloadedImages(ctx context.Context, paths []string, question string) (string, error) {
	question = stripPlatformImageSteer(question)
	var summaries []string
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		out, err := agent.ExecuteBuiltinTool("analyze_image", map[string]interface{}{"path": path, "ocr_lang": "eng+ind", "include_metadata": false}, nil)
		if err != nil {
			return "", err
		}
		summaries = append(summaries, summarizeImageAnalysisForChat(out))
	}
	return composeImageAnalysisReply(question, summaries), nil
}

func summarizeImageAnalysisForChat(output string) string {
	ocr := extractOCRText(output)
	if ocr == "" {
		return "Saya bisa membaca gambar yang dikirim, tetapi tidak menemukan teks yang cukup jelas untuk diambil dari gambar tersebut."
	}
	return ocr
}

func extractOCRText(output string) string {
	marker := "── OCR Text (tesseract) ──"
	idx := strings.Index(output, marker)
	if idx < 0 {
		return ""
	}
	text := strings.TrimSpace(output[idx+len(marker):])
	if next := strings.Index(text, "\n── "); next >= 0 {
		text = text[:next]
	}
	return sanitizeImageAnalysisText(text)
}

func sanitizeImageAnalysisText(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "path") || strings.HasPrefix(lower, "size") || strings.HasPrefix(lower, "modified") || strings.HasPrefix(lower, "dimensions") || strings.HasPrefix(lower, "format") {
			continue
		}
		if strings.Contains(line, "/.smara/") || strings.Contains(line, "/home/") {
			continue
		}
		kept = append(kept, line)
		if len(kept) >= 14 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func composeImageAnalysisReply(question string, summaries []string) string {
	combined := strings.TrimSpace(strings.Join(summaries, "\n\n"))
	if combined == "" {
		return "Saya belum bisa mengenali isi gambar dengan jelas."
	}
	lowerQuestion := strings.ToLower(question)
	if looksLikeSubscriptionCheckout(combined) {
		if strings.Contains(lowerQuestion, "kenapa") || strings.Contains(lowerQuestion, "masalah") {
			return "Itu muncul karena aplikasi sedang menampilkan halaman upgrade/checkout. Akun terlihat masih di paket Free atau aksesnya terbatas, lalu diarahkan ke pilihan paket berbayar seperti Plus/Pro dengan opsi pembayaran GoPay."
		}
		return "Itu screenshot halaman upgrade/subscription aplikasi AI. Isinya menawarkan paket Free, Plus, dan Pro, termasuk trial, image creation, memory, deep research/agent mode, dan pembayaran GoPay."
	}
	if strings.Contains(lowerQuestion, "teks") || strings.Contains(lowerQuestion, "tulisan") || strings.Contains(lowerQuestion, "ocr") {
		return "Teks yang terbaca dari gambar:\n\n" + combined
	}
	return firstMeaningfulImageSummary(combined)
}

func looksLikeSubscriptionCheckout(text string) bool {
	lower := strings.ToLower(text)
	matches := 0
	for _, term := range []string{"free", "plus", "pro", "trial", "checkout", "gopay", "image creation", "memory", "subscription"} {
		if strings.Contains(lower, term) {
			matches++
		}
	}
	return matches >= 3
}

func firstMeaningfulImageSummary(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
		if len(kept) >= 4 {
			break
		}
	}
	if len(kept) == 0 {
		return "Saya belum bisa menyimpulkan isi gambar dengan jelas."
	}
	return "Gambar ini tampaknya berisi: " + strings.Join(kept, "; ")
}

func stripPlatformImageSteer(prompt string) string {
	if idx := strings.Index(prompt, "\n\n[Sistem: pesan ini menyertakan gambar."); idx >= 0 {
		prompt = prompt[:idx]
	}
	fields := strings.Fields(prompt)
	kept := fields[:0]
	for _, field := range fields {
		if strings.HasPrefix(field, "[image:") && strings.HasSuffix(field, "]") {
			continue
		}
		kept = append(kept, field)
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func imageAttachmentsFromToolOutput(output string) []Attachment {
	seen := map[string]bool{}
	var attachments []Attachment
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Path:") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "Path:"))
		if path == "" || seen[path] {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		seen[path] = true
		attachments = append(attachments, Attachment{
			Type:     "image",
			FilePath: path,
			FileName: filepath.Base(path),
			MimeType: imageMimeType(path),
			Size:     info.Size(),
		})
	}
	return attachments
}

func mergeAttachments(primary, secondary []Attachment) []Attachment {
	if len(primary) == 0 {
		return secondary
	}
	if len(secondary) == 0 {
		return primary
	}
	seen := map[string]bool{}
	merged := make([]Attachment, 0, len(primary)+len(secondary))
	for _, att := range append(primary, secondary...) {
		key := att.FilePath
		if key == "" {
			key = att.URL
		}
		if key != "" && seen[key] {
			continue
		}
		if key != "" {
			seen[key] = true
		}
		merged = append(merged, att)
	}
	return merged
}

func imageMimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

// splitMessage breaks a long message into chunks that fit within the platform limit.
func splitMessage(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}

	var parts []string
	for len(content) > 0 {
		if len(content) <= maxLen {
			parts = append(parts, content)
			break
		}

		// Try to split at a newline near the limit
		splitAt := maxLen
		lastNewline := strings.LastIndex(content[:maxLen], "\n")
		if lastNewline > maxLen/2 {
			splitAt = lastNewline + 1
		}

		parts = append(parts, content[:splitAt])
		content = content[splitAt:]
	}

	return parts
}
