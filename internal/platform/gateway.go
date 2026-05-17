package platform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
)

// maxMessageLength is the maximum length of a single message on most platforms.
const maxMessageLength = 4000

// promptTimeout is the maximum time allowed for a single prompt processing
// (agentic loop with tool calls). Default 10 minutes; configurable per-instance
// via Gateway.SetPromptTimeout (and `platform_prompt_timeout` in config.yaml).
const defaultPromptTimeout = 10 * time.Minute

// progressHeartbeatInterval is how often the status message shows a progress
// update so users know the bot is still working rather than hung.
const progressHeartbeatInterval = 15 * time.Second

// Gateway routes incoming messages from platform adapters to the Smara supervisor
// and sends responses back. It manages per-channel sessions, authentication,
// and rate limiting.
type Gateway struct {
	adapters      map[string]PlatformAdapter
	supervisor    *agent.Supervisor
	sessions      map[string]*PlatformSession // channelID → session
	auth          *AuthManager
	rateLimiter   *RateLimiter
	metrics       *metrics.MetricsCollector
	promptTimeout time.Duration
	mu            sync.RWMutex
}

// NewGateway creates a new Gateway with the given supervisor.
func NewGateway(supervisor *agent.Supervisor) *Gateway {
	return &Gateway{
		adapters:      make(map[string]PlatformAdapter),
		supervisor:    supervisor,
		sessions:      make(map[string]*PlatformSession),
		auth:          NewAuthManager(),
		rateLimiter:   NewRateLimiter(RateLimitConfig{RequestsPerMinute: 20, BurstSize: 5}),
		promptTimeout: defaultPromptTimeout,
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

// SetPromptTimeout overrides the default per-prompt timeout. Pass 0 to keep
// the default (10 minutes). Must be called before Start.
func (g *Gateway) SetPromptTimeout(d time.Duration) {
	if d <= 0 {
		g.promptTimeout = defaultPromptTimeout
		return
	}
	g.promptTimeout = d
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
	log.Printf("[gateway] Incoming message from %s/%s (user=%s): %q", msg.Platform, msg.ChannelID, msg.UserID, msg.Content)

	// 1. Auth check
	if !g.auth.IsAllowed(msg.Platform, msg.UserID) {
		log.Printf("[gateway] Auth denied for %s/%s", msg.Platform, msg.UserID)
		return g.sendReply(ctx, msg, "⛔ Akses ditolak. Hubungi admin untuk mendapatkan akses.")
	}

	// 2. Rate limit check
	if !g.rateLimiter.Allow(msg.UserID) {
		log.Printf("[gateway] Rate limit hit for %s", msg.UserID)
		return g.sendReply(ctx, msg, "⏳ Rate limit tercapai. Coba lagi dalam beberapa saat.")
	}

	// 3. Record incoming message metric
	if g.metrics != nil {
		g.metrics.RecordMessageIn(msg.Platform, msg.UserID, msg.Username)
	}

	// 4. Handle commands
	if msg.IsCommand {
		log.Printf("[gateway] Processing command /%s", msg.Command)
		return g.handleCommand(ctx, msg)
	}

	// 5. Process as prompt
	log.Printf("[gateway] Processing prompt via supervisor (mode=%s)", g.supervisor.GetMode())
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
// It provides real-time UX feedback by sending and updating a status message.
func (g *Gateway) processPrompt(ctx context.Context, msg IncomingMessage) error {
	g.mu.RLock()
	adapter, ok := g.adapters[msg.Platform]
	g.mu.RUnlock()
	if !ok {
		return fmt.Errorf("adapter tidak ditemukan: %s", msg.Platform)
	}

	// 0. Download image attachments (if any) and inject [image:/path] tokens
	// into the prompt. Adapters declare attachment download capability via
	// the AttachmentDownloader interface.
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
			log.Printf("[gateway] adapter %s belum support download attachment, %d attachment di-skip",
				msg.Platform, len(msg.Attachments))
		}
	}

	if len(downloadedImages) > 0 && isImageAnalysisPrompt(msg.Content) {
		_ = adapter.SendTyping(ctx, msg.ChannelID)
		output, err := analyzeDownloadedImages(ctx, downloadedImages, msg.Content)
		if err != nil {
			return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
		}
		return g.sendReply(ctx, msg, output)
	}

	if isImageGenerationPrompt(msg.Content) {
		_ = adapter.SendTyping(ctx, msg.ChannelID)
		output, err := agent.ExecuteBuiltinTool("generate_image", map[string]interface{}{"prompt": msg.Content}, nil)
		if err != nil {
			return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
		}
		return g.sendReplyWithAttachments(ctx, msg, output, imageAttachmentsFromToolOutput(output))
	}

	// 1. Send initial status message
	statusMsg := OutgoingMessage{Content: RenderStatusMessage(msg.Platform, "thinking", "Sedang menyiapkan jawaban terbaik...", 0), Format: FormatMarkdown}
	statusMsgID, err := adapter.SendMessageWithID(ctx, msg.ChannelID, statusMsg)
	if err != nil {
		// Fallback: just send typing indicator if status message fails
		_ = adapter.SendTyping(ctx, msg.ChannelID)
		statusMsgID = ""
	}

	// 2. Track current status for live updates
	var statusMu sync.Mutex
	currentStatus := statusMsg.Content
	lastEditTime := time.Now()

	updateStatus := func(newStatus string) {
		statusMu.Lock()
		defer statusMu.Unlock()
		if statusMsgID == "" || newStatus == currentStatus {
			return
		}
		// Rate limit edits: min 1.5s between edits (Telegram API limit)
		if time.Since(lastEditTime) < 1500*time.Millisecond {
			return
		}
		currentStatus = newStatus
		lastEditTime = time.Now()
		editMsg := OutgoingMessage{Content: newStatus, Format: FormatMarkdown}
		_ = adapter.EditMessage(ctx, msg.ChannelID, statusMsgID, editMsg)
	}

	startHeartbeat := time.Now()

	var generatedMu sync.Mutex
	var generatedAttachments []Attachment
	currentTool := ""

	// 3. Set up supervisor callbacks for phase changes
	g.supervisor.SetCallback(agent.AgenticCallback{
		OnPhaseChange: func(phase, description string) {
			updateStatus(RenderStatusMessage(msg.Platform, phase, description, time.Since(startHeartbeat)))
		},
		OnToolCall: func(server, tool string, args map[string]interface{}) {
			generatedMu.Lock()
			currentTool = tool
			generatedMu.Unlock()

			toolName := tool
			if len(toolName) > 30 {
				toolName = toolName[:30] + "…"
			}
			updateStatus(RenderStatusMessage(msg.Platform, "tool_call", "Menjalankan: "+toolName, time.Since(startHeartbeat)))
		},
		OnToolResult: func(output string) {
			generatedMu.Lock()
			if currentTool == "generate_image" {
				generatedAttachments = append(generatedAttachments, imageAttachmentsFromToolOutput(output)...)
			}
			generatedMu.Unlock()
			updateStatus(RenderStatusMessage(msg.Platform, "analyzing", "Menganalisis hasil tool...", time.Since(startHeartbeat)))
		},
		OnIteration: func(current, max int) {
			updateStatus(RenderStatusMessage(msg.Platform, "iteration", fmt.Sprintf("Iterasi %d/%d", current, max), time.Since(startHeartbeat)))
		},
	})

	// 4. Continuous typing indicator + timeout
	timeout := g.promptTimeout
	if timeout <= 0 {
		timeout = defaultPromptTimeout
	}
	promptCtx, promptCancel := context.WithTimeout(ctx, timeout)
	defer promptCancel()

	// Keep sending typing indicator every 4 seconds
	typingDone := make(chan struct{})
	go func() {
		defer close(typingDone)
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-promptCtx.Done():
				return
			case <-ticker.C:
				_ = adapter.SendTyping(promptCtx, msg.ChannelID)
			}
		}
	}()

	// Heartbeat progress updates: every 15s, update status message with elapsed
	// time so the user knows the bot is still working (not frozen).
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(progressHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-promptCtx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(startHeartbeat)
				secs := int(elapsed.Seconds())
				remaining := int(timeout.Seconds()) - secs
				if remaining < 0 {
					remaining = 0
				}
				updateStatus(RenderStatusMessage(msg.Platform, "processing", fmt.Sprintf("Masih memproses • %ds berlalu • est. maks %ds tersisa", secs, remaining), elapsed))
			}
		}
	}()

	// 5. Process via supervisor
	log.Printf("[gateway] Calling supervisor.ProcessPrompt: %q", msg.Content)
	startTime := time.Now()
	result, err := g.supervisor.ProcessPrompt(promptCtx, msg.Content)
	latencyMs := time.Since(startTime).Milliseconds()
	promptErr := promptCtx.Err()
	log.Printf("[gateway] supervisor.ProcessPrompt done in %dms, err=%v", latencyMs, err)

	// Stop typing indicator & heartbeat
	promptCancel()
	<-typingDone
	<-heartbeatDone

	// Clear callbacks
	g.supervisor.SetCallback(agent.AgenticCallback{})

	// 6. Delete status message (best effort)
	if statusMsgID != "" {
		deleteMsg := OutgoingMessage{Content: "⏳", Format: FormatPlain}
		_ = adapter.EditMessage(ctx, msg.ChannelID, statusMsgID, deleteMsg)
	}

	// 7. Handle errors
	if err != nil {
		log.Printf("[gateway] supervisor error: %v", err)
		if g.metrics != nil {
			g.metrics.RecordError(msg.Platform, err.Error())
		}
		errText := err.Error()
		if errors.Is(promptErr, context.DeadlineExceeded) {
			totalSec := int(time.Since(startHeartbeat).Seconds())
			errText = fmt.Sprintf(
				"Timeout: proses berjalan %ds dan melewati batas maksimal %ds.\n\n"+
					"Tips:\n"+
					"• Pecah tugas jadi langkah-langkah lebih kecil\n"+
					"• Gunakan perintah spesifik (mis. \"cek service X\" daripada \"cek semua\")\n"+
					"• Untuk task panjang (multi-SSH, deep reasoning), naikkan timeout di config:\n"+
					"    platform_prompt_timeout: 1200   # 20 menit\n"+
					"  lalu restart bot.\n"+
					"• Atau coba lagi dengan /ask untuk respons singkat tanpa tool",
				totalSec, int(timeout.Seconds()),
			)
		}
		// Update status message with error instead of sending new one
		if statusMsgID != "" {
			editMsg := OutgoingMessage{Content: "❌ Error: " + errText, Format: FormatPlain}
			return adapter.EditMessage(ctx, msg.ChannelID, statusMsgID, editMsg)
		}
		return g.sendReply(ctx, msg, "❌ Error: "+errText)
	}

	respPreview := result.Response
	if len(respPreview) > 200 {
		respPreview = respPreview[:200] + "..."
	}
	log.Printf("[gateway] supervisor result: response=%q tools=%d", respPreview, len(result.ToolsExecuted))

	// 8. Record metrics
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

	// 9. Build final response with stats footer
	// Defense-in-depth: sanitize DSML here too. The supervisor strips DSML
	// during the agentic loop, but edge cases (max-iterations fallback,
	// models emitting malformed DSML near the final answer) can still leak
	// raw tool-call markup. sanitizeDSML is idempotent and cheap.
	finalResp := sanitizeDSML(result.Response)
	if finalResp == "" {
		// If the supervisor's final response was entirely DSML that we
		// just stripped, fall back to the intermediate thoughts (last
		// few reasoning steps) + tools executed so the user gets
		// something actionable instead of a misleading "Selesai".
		finalResp = buildFallbackSummary(result.Thoughts, result.ToolsExecuted)
	}
	finalResp = RenderPlatformResponse(
		msg.Platform,
		finalResp,
		g.supervisor.GetModel(),
		time.Duration(latencyMs)*time.Millisecond,
		len(result.ToolsExecuted),
		result.InputTokens,
		result.OutputTokens,
	)
	generatedMu.Lock()
	attachments := append([]Attachment(nil), generatedAttachments...)
	generatedMu.Unlock()

	// 10. Update status message with final response, or send new message
	if statusMsgID != "" {
		// If response is short enough and has no file attachments, edit the status message
		if len(attachments) == 0 && len(finalResp) <= maxMessageLength {
			editMsg := OutgoingMessage{Content: finalResp, Format: FormatMarkdown}
			if err := adapter.EditMessage(ctx, msg.ChannelID, statusMsgID, editMsg); err == nil {
				return nil
			}
		}
		// For attachments, long responses, or edit failure, finish status and send normally
		editMsg := OutgoingMessage{Content: "✅", Format: FormatPlain}
		_ = adapter.EditMessage(ctx, msg.ChannelID, statusMsgID, editMsg)
	}

	log.Printf("[gateway] Sending reply to %s/%s", msg.Platform, msg.ChannelID)
	return g.sendReplyWithAttachments(ctx, msg, finalResp, attachments)
}

// phaseEmoji returns an emoji for a processing phase.
func phaseEmoji(phase string) string {
	switch strings.ToLower(phase) {
	case "thinking":
		return "🤔"
	case "tool_call", "tool_execution", "executing":
		return "🔧"
	case "analyzing", "analysis":
		return "🔍"
	case "responding", "response":
		return "📝"
	case "planning":
		return "📋"
	case "searching", "search":
		return "🔎"
	case "writing", "coding":
		return "💻"
	default:
		return "⚡"
	}
}

// sanitizeDSML strips leaked DSML tool-calling markup from AI responses.
// This is a defense-in-depth safeguard; the supervisor should already strip
// DSML before returning, but some edge cases (e.g., max-iterations fallback,
// models emitting malformed DSML near the answer) can let fragments through.
// We delegate to llm.ExtractToolCallsFromContent which handles all known
// format variants including double full-width pipes (U+FF5C).
func sanitizeDSML(text string) string {
	if text == "" {
		return text
	}
	_, cleaned := llm.ExtractToolCallsFromContent(text)
	return strings.TrimSpace(cleaned)
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

	// Sanitize DSML markup before sending
	content = sanitizeDSML(content)
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
	var sb strings.Builder
	question = stripPlatformImageSteer(question)
	sb.WriteString("📷 Hasil analisis gambar")
	if question != "" {
		sb.WriteString(" untuk: ")
		sb.WriteString(question)
	}
	sb.WriteString("\n\n")
	for i, path := range paths {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		out, err := agent.ExecuteBuiltinTool("analyze_image", map[string]interface{}{"path": path, "ocr_lang": "eng+ind", "include_metadata": true}, nil)
		if err != nil {
			return "", err
		}
		if len(paths) > 1 {
			sb.WriteString(fmt.Sprintf("## Gambar %d\n\n", i+1))
		}
		sb.WriteString(out)
		if !strings.HasSuffix(out, "\n") {
			sb.WriteString("\n")
		}
		if i < len(paths)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
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

// buildFallbackSummary composes a useful reply when the supervisor's final
// response is empty (typically because the model emitted only DSML tool
// calls that got stripped, or hit the max-iteration cap). Returning an
// empty string in Telegram looks like the bot froze; this function lets
// the user see the intermediate progress instead.
func buildFallbackSummary(thoughts []string, tools []string) string {
	var sb strings.Builder
	sb.WriteString("⚠ Smara tidak menghasilkan jawaban final yang jelas — tool loop berhenti tanpa kesimpulan.\n\n")

	// Keep the last 3 non-empty thoughts.
	kept := 0
	for i := len(thoughts) - 1; i >= 0 && kept < 3; i-- {
		t := strings.TrimSpace(thoughts[i])
		t = sanitizeDSML(t)
		if t == "" {
			continue
		}
		if len(t) > 260 {
			t = t[:260] + "…"
		}
		sb.WriteString("• " + t + "\n")
		kept++
	}
	if kept == 0 {
		sb.WriteString("(Tidak ada komentar intermediate yang bisa ditampilkan.)\n")
	}

	if len(tools) > 0 {
		seen := map[string]bool{}
		var list []string
		for _, t := range tools {
			if !seen[t] {
				seen[t] = true
				list = append(list, t)
				if len(list) >= 8 {
					break
				}
			}
		}
		sb.WriteString(fmt.Sprintf("\n🔧 Tools dijalankan (%d unik dari %d total): %s", len(seen), len(tools), strings.Join(list, ", ")))
		if len(seen) > 8 {
			sb.WriteString(fmt.Sprintf(" +%d lainnya", len(seen)-8))
		}
	}

	sb.WriteString("\n\nSaran: pecah permintaan jadi 1-2 langkah yang lebih spesifik, atau kirim `/clear` lalu coba ulang.")
	return sb.String()
}
