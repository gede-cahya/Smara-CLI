package platform

import (
	"context"
	"errors"
	"fmt"
	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/browser"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/gede-cahya/Smara-CLI/internal/metrics"
	"github.com/gede-cahya/Smara-CLI/internal/orchestration"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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
	adapters        map[string]PlatformAdapter
	supervisor      *agent.Supervisor
	sessions        map[string]*PlatformSession // platform/channel/user → supervisor session binding
	auth            *AuthManager
	rateLimiter     *RateLimiter
	metrics         *metrics.MetricsCollector
	promptTimeout   time.Duration
	sensitiveGuards map[string]SensitiveDataGuard
	mu              sync.RWMutex
	promptMu        sync.Mutex // supervisor has global current session/history; serialize platform prompts
}
// NewGateway creates a new Gateway with the given supervisor.
func NewGateway(supervisor *agent.Supervisor) *Gateway {
	return &Gateway{
		adapters:        make(map[string]PlatformAdapter),
		supervisor:      supervisor,
		sessions:        make(map[string]*PlatformSession),
		auth:            NewAuthManager(),
		rateLimiter:     NewRateLimiter(RateLimitConfig{RequestsPerMinute: 20, BurstSize: 5}),
		promptTimeout:   defaultPromptTimeout,
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
	log.Printf("[gateway] Incoming message from %s/%s (user=%s): %q", msg.Platform, msg.ChannelID, msg.UserID, redactSensitiveLogContent(msg.Content))

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
		log.Printf("[gateway] Processing command /%s", msg.Command)
		return g.handleCommand(ctx, msg)
	}

	// 5. Process as prompt. Session binding and mode-specific routing happen
	// inside processPrompt, after the correct platform session is selected.
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
/mode <ask|rush|plan|test|image|workflow> — Ganti mode agen
/prd [idea] — Buat PRD interaktif dengan button dan file Markdown
/mcp — Lihat MCP tools
/memory_autolink [aggressive|smart] — Bangun auto-link memory graph
/autolink [aggressive|smart] — Alias memory autolink
/clear — Reset percakapan
/help — Bantuan
/help — Bantuan
Atau langsung ketik pesan untuk memulai percakapan.`
		return g.sendReply(ctx, msg, welcome)

	case "help":
		help := `📖 *Bantuan Smara Bot*

/ask <prompt> — Kirim prompt ke Smara
/mode — Lihat mode saat ini
/mode <ask|rush|plan|test|image|workflow> — Ganti mode
/prd [idea] — Buat PRD interaktif dengan button dan file Markdown
/mcp — Daftar MCP tools
/memory_autolink [aggressive|smart] — Bangun auto-link memory graph
/autolink [aggressive|smart] — Alias memory autolink
/clear — Reset history percakapan
/help — Tampilkan pesan ini

💡 Anda juga bisa langsung mengetik pesan tanpa perintah.`
		return g.sendReply(ctx, msg, help)

	case "ask":
		if len(msg.CommandArgs) == 0 {
			return g.sendReply(ctx, msg, "❌ Gunakan: /ask <pertanyaan>")
		}
		// Reconstruct prompt from args and pass it through the same explicit
		// custom-workflow router used by normal adapter messages before falling
		// back to generic prompt processing. This keeps `/ask jalankan custom
		// workflow ...` on the saved workflow runner instead of the generic
		// auto-orchestration path.
		prompt := strings.Join(msg.CommandArgs, " ")
		promptMsg := msg
		promptMsg.Content = prompt
		if g.supervisor.GetMode() == agent.ModeWorkflow {
			if response, handled, err := g.tryRunCustomWorkflowPrompt(promptMsg); handled {
				if err != nil {
					return g.sendReply(ctx, msg, "❌ "+err.Error())
				}
				return g.sendReply(ctx, msg, response)
			}
		}
		return g.processPrompt(ctx, promptMsg)

	case "prd":
		return g.sendReply(ctx, msg, "ℹ️ Fitur PRD interaktif tersedia di Discord slash command: `/smara prd idea:<ide produk>`. Gunakan slash command agar button Discord bisa muncul dan PRD dikirim sebagai file Markdown.")

	case "mode":
		if len(msg.CommandArgs) == 0 {
			// Show current mode
			current := g.supervisor.GetMode()
			info := agent.GetModeInfo(current)
			reply := fmt.Sprintf("Mode saat ini: %s %s\n\n%s\n\nGunakan /mode <ask|rush|plan|test|image|workflow> untuk mengganti.", info.Emoji, info.Label, info.Description)
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

	case "memory_autolink", "autolink":
		return g.handleMemoryAutolinkCommand(ctx, msg)

	default:
		return g.sendReply(ctx, msg, fmt.Sprintf("❓ Perintah tidak dikenal: /%s\nKetik /help untuk bantuan.", msg.Command))
	}
}

func (g *Gateway) handleMemoryAutolinkCommand(ctx context.Context, msg IncomingMessage) error {
	store, ok := g.supervisor.GetMemoryStore().(*memory.SQLiteStore)
	if !ok || store == nil {
		return g.sendReply(ctx, msg, "❌ Memory store tidak mendukung autolink graph.")
	}

	strategy := "aggressive"
	threshold := 0.28
	topK := 10
	for i := 0; i < len(msg.CommandArgs); i++ {
		arg := strings.TrimSpace(msg.CommandArgs[i])
		switch strings.ToLower(arg) {
		case "aggressive", "smart", "semantic", "lexical":
			strategy = strings.ToLower(arg)
		case "--strategy":
			if i+1 < len(msg.CommandArgs) {
				strategy = strings.ToLower(msg.CommandArgs[i+1])
				i++
			}
		case "--threshold":
			if i+1 < len(msg.CommandArgs) {
				if v, err := strconv.ParseFloat(msg.CommandArgs[i+1], 64); err == nil {
					threshold = v
				}
				i++
			}
		case "--top-k", "--top_k":
			if i+1 < len(msg.CommandArgs) {
				if v, err := strconv.Atoi(msg.CommandArgs[i+1]); err == nil {
					topK = v
				}
				i++
			}
		}
	}
	if strategy != "aggressive" {
		if threshold == 0.28 {
			threshold = 0.78
		}
		if topK == 10 {
			topK = 5
		}
	}

	report, err := store.AutoLinkSmart(memory.AutoLinkOptions{
		WorkspaceID:    g.supervisor.GetWorkspaceID(),
		Threshold:      threshold,
		MaxPerNode:     topK,
		Replace:        true,
		Strategy:       strategy,
		HubLinks:       strategy == "aggressive",
		AttachIsolated: strategy == "aggressive",
		HubThreshold:   0.18,
	})
	if err != nil {
		return g.sendReply(ctx, msg, "❌ Memory autolink gagal: "+err.Error())
	}

	reply := fmt.Sprintf("✅ Memory autolink selesai\n\nMode: %s\nCreated: %d\nScanned: %d\nThreshold: %.2f\nTop-K: %d\nAttached isolated: %d",
		report.Mode, report.Created, report.MemoriesScanned, report.Threshold, report.TopK, report.AttachedIsolated)
	return g.sendReply(ctx, msg, reply)
}

func (g *Gateway) tryRunCustomWorkflowPrompt(msg IncomingMessage) (string, bool, error) {
	candidate, ok := extractWorkflowRunName(msg.Content)
	if !ok {
		return "", false, nil
	}
	cw, matched, err := findCustomWorkflow(candidate)
	if err != nil || cw == nil {
		return "", false, nil
	}
	log.Printf("[gateway] Routing %s/%s prompt to parallel custom workflow: %s", msg.Platform, msg.ChannelID, matched)
	result, err := workflow.RunCustomWorkflow(g.supervisor, g.supervisor.GetProvider(), cw)
	if err != nil {
		return "", true, fmt.Errorf("gagal menjalankan custom workflow '%s': %w", matched, err)
	}
	return formatWorkflowRunResponse(matched, result), true, nil
}

func extractWorkflowRunName(prompt string) (string, bool) {
	text := strings.Trim(strings.TrimSpace(prompt), " \t\n\r`'\".,!")
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	prefixes := []string{"jalankan custom workflow ", "jalankan workflow ", "run custom workflow ", "run workflow ", "execute custom workflow ", "execute workflow ", "mulai custom workflow ", "mulai workflow ", "start custom workflow ", "start workflow ", "jalankan ", "run ", "execute ", "mulai ", "start "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			name := strings.Trim(strings.TrimSpace(text[len(prefix):]), " \t\n\r`'\".,!")
			return name, name != ""
		}
	}
	return "", false
}

func findCustomWorkflow(candidate string) (*workflow.CustomWorkflow, string, error) {
	workflows, err := workflow.LoadAllCustomWorkflows()
	if err != nil {
		return nil, "", err
	}
	norm := normalizeWorkflowName(candidate)
	for _, cw := range workflows {
		if normalizeWorkflowName(cw.Name) == norm {
			return cw, cw.Name, nil
		}
	}
	for _, cw := range workflows {
		if strings.Contains(normalizeWorkflowName(cw.Name), norm) || strings.Contains(norm, normalizeWorkflowName(cw.Name)) {
			return cw, cw.Name, nil
		}
		for _, a := range cw.Agents {
			if normalizeWorkflowName(a.Role) == norm || strings.Contains(normalizeWorkflowName(a.Role), norm) {
				return cw, cw.Name, nil
			}
		}
	}
	return nil, "", nil
}

func normalizeWorkflowName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }), "-")
}

func formatWorkflowRunResponse(name string, result *workflow.CustomWorkflowResult) string {
	if result == nil {
		return "✅ Workflow selesai."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ Parallel custom workflow '%s' selesai\n", name))
	if result.FinalSummary != "" {
		sb.WriteString("\n")
		sb.WriteString(result.FinalSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nParallel execution:\n")
	if result.ParallelExecution {
		sb.WriteString("- Ya, minimal satu wave berisi beberapa role yang berjalan bersamaan sesuai dependency.\n")
	} else {
		sb.WriteString("- Tidak ada wave paralel; dependency workflow membuat eksekusi serial atau hanya ada satu role per wave.\n")
	}
	if waves := formatWorkflowWavesForGateway(result.Waves); waves != "" {
		sb.WriteString(waves)
	}
	if result.QAResult.Status != "" {
		sb.WriteString(fmt.Sprintf("\nQA: %s", result.QAResult.Status))
		if len(result.QAResult.Issues) > 0 {
			sb.WriteString(fmt.Sprintf(" (%d issue(s))", len(result.QAResult.Issues)))
		}
		sb.WriteString("\n")
	}
	if result.ProjectPath != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectPath))
	}
	return strings.TrimSpace(sb.String())
}

func formatWorkflowWavesForGateway(waves [][]string) string {
	if len(waves) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, wave := range waves {
		if len(wave) == 0 {
			continue
		}
		roles := append([]string(nil), wave...)
		sort.Strings(roles)
		mode := "serial"
		if len(roles) > 1 {
			mode = "parallel"
		}
		sb.WriteString(fmt.Sprintf("- Wave %d (%s): %s\n", i+1, mode, strings.Join(roles, ", ")))
	}
	return sb.String()
}

func (g *Gateway) tryRunAgentSwarmWorkflowPrompt(msg IncomingMessage) (string, bool, error) {
	if g.supervisor.GetMode() != agent.ModeParallel || !orchestration.IsAgentSwarmWorkflowPrompt(msg.Content) {
		return "", false, nil
	}
	log.Printf("[gateway] Routing %s/%s prompt to Agent Swarm Workflow", msg.Platform, msg.ChannelID)
	result, err := workflow.RunWorkflow(g.supervisor, g.supervisor.GetProvider(), msg.Content)
	if err != nil {
		return "", true, fmt.Errorf("Agent Swarm Workflow gagal: %w", err)
	}
	return formatAgentSwarmWorkflowResponse(result), true, nil
}

func formatAgentSwarmWorkflowResponse(result *workflow.WorkflowResult) string {
	if result == nil {
		return "✅ Agent Swarm Workflow selesai."
	}
	summary := strings.TrimSpace(result.FinalSummary)
	if summary == "" {
		summary = "Workflow selesai tanpa ringkasan tambahan."
	}
	var sb strings.Builder
	sb.WriteString("✅ Agent Swarm Workflow selesai\n\n")
	sb.WriteString("Mode: multi-agent task decomposition, agent spawning, parallel wave execution, result merge, QA.\n\n")
	sb.WriteString(summary)
	sb.WriteString("\n\nParallel execution:\n")
	if result.ParallelExecution {
		sb.WriteString(fmt.Sprintf("- Ya, wave paralel aktif dengan max concurrency %d.\n", result.MaxConcurrency))
	} else {
		sb.WriteString(fmt.Sprintf("- Tidak ada wave paralel; eksekusi serial atau hanya satu agent per wave. Max concurrency %d.\n", result.MaxConcurrency))
	}
	if waves := formatWorkflowWavesForGateway(result.ExecutionWaves); waves != "" {
		sb.WriteString(waves)
	}
	if result.QAResult.Status != "" {
		sb.WriteString(fmt.Sprintf("QA: %s", result.QAResult.Status))
		if len(result.QAResult.Issues) > 0 {
			sb.WriteString(fmt.Sprintf(" (%d issue)", len(result.QAResult.Issues)))
		}
		sb.WriteString("\n")
	}
	if result.ProjectPath != "" {
		sb.WriteString(fmt.Sprintf("Project: %s", result.ProjectPath))
	}
	return strings.TrimSpace(sb.String())
}

func (g *Gateway) tryRunAutoParallelOrchestrationPrompt(msg IncomingMessage) (string, bool, error) {
	if !shouldAutoParallelOrchestrate(msg.Content, g.supervisor.GetMode()) {
		return "", false, nil
	}
	log.Printf("[gateway] Auto-routing complex %s/%s prompt to parallel orchestration", msg.Platform, msg.ChannelID)
	result, err := workflow.RunWorkflow(g.supervisor, g.supervisor.GetProvider(), msg.Content)
	if err != nil {
		return "", true, fmt.Errorf("auto parallel orchestration gagal: %w", err)
	}
	return formatAutoWorkflowResponse(result), true, nil
}

func shouldAutoParallelOrchestrate(prompt string, mode agent.Mode) bool {
	return orchestration.ShouldAutoParallelOrchestrate(prompt, mode)
}

func formatAutoWorkflowResponse(result *workflow.WorkflowResult) string {
	if result == nil {
		return "✅ Auto parallel orchestration selesai."
	}
	summary := strings.TrimSpace(result.FinalSummary)
	if summary == "" {
		summary = "Workflow selesai tanpa ringkasan tambahan."
	}
	return fmt.Sprintf("✅ Auto parallel orchestration selesai\n\n%s\nProject: %s", summary, result.ProjectPath)
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

	if denied, denyMessage := g.checkSensitiveDataAccess(msg); denied {
		log.Printf("[gateway] Sensitive data request denied for %s user=%s", msg.Platform, msg.UserID)
		return g.sendReply(ctx, msg, denyMessage)
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
		log.Printf("[gateway] image analysis fast-path matched for %s/%s images=%d", msg.Platform, msg.ChannelID, len(downloadedImages))
		_ = adapter.SendTyping(ctx, msg.ChannelID)
		output, err := analyzeDownloadedImages(ctx, downloadedImages, msg.Content)
		if err != nil {
			return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
		}
		return g.sendReply(ctx, msg, output)
	}

	if g.supervisor.GetMode() == agent.ModeImage && isImageGenerationPrompt(msg.Content) {
		log.Printf("[gateway] image generation fast-path matched for %s/%s: %q", msg.Platform, msg.ChannelID, redactSensitiveLogContent(msg.Content))
		_ = adapter.SendTyping(ctx, msg.ChannelID)
		output, err := agent.ExecuteBuiltinTool("generate_image", map[string]interface{}{"prompt": msg.Content}, nil)
		if err != nil {
			return g.sendReply(ctx, msg, "❌ Error: "+err.Error())
		}
		return g.sendReplyWithAttachments(ctx, msg, output, imageAttachmentsFromToolOutput(output))
	}

	if browser.IsBrowserPrompt(msg.Content) {
		return g.processBrowserPrompt(ctx, msg)
	}

	if g.supervisor.GetMode() == agent.ModeWorkflow {
		if response, handled, err := g.tryRunCustomWorkflowPrompt(msg); handled {
			if err != nil {
				return g.sendReply(ctx, msg, "❌ "+err.Error())
			}
			return g.sendReply(ctx, msg, response)
		}
	}

	if response, handled, err := g.tryRunAgentSwarmWorkflowPrompt(msg); handled {
		if err != nil {
			return g.sendReply(ctx, msg, "❌ "+err.Error())
		}
		return g.sendReply(ctx, msg, response)
	}

	if response, handled, err := g.tryRunAutoParallelOrchestrationPrompt(msg); handled {
		if err != nil {
			return g.sendReply(ctx, msg, "❌ "+err.Error())
		}
		return g.sendReply(ctx, msg, response)
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

	// 5. Process via supervisor. The supervisor keeps a single in-memory
	// current session/history, so platform prompts must be serialized and the
	// correct platform-bound session must be selected immediately before the
	// call. Without this, Telegram chats/users can inherit unrelated CLI or
	// platform context.
	log.Printf("[gateway] Calling supervisor.ProcessPrompt: %q", redactSensitiveLogContent(msg.Content))
	startTime := time.Now()
	g.promptMu.Lock()
	if err := g.ensurePlatformSessionLocked(msg); err != nil {
		g.promptMu.Unlock()
		promptCancel()
		<-typingDone
		<-heartbeatDone
		return g.sendReply(ctx, msg, "❌ Error menyiapkan session: "+err.Error())
	}
	result, err := g.supervisor.ProcessPrompt(promptCtx, msg.Content)
	g.promptMu.Unlock()
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
// Fixed: now delegates to llm.SanitizeForUser which handles fullwidth pipes,
// partial/truncated tags, and aggressive residual cleaning.
func sanitizeDSML(text string) string {
	if text == "" {
		return text
	}
	return llm.SanitizeForUser(text)
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
	attachments = mergeAttachments(attachments, autoVisualAttachmentsFromResponse(content))

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
	if isSoftwareImageFeaturePrompt(p) {
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

func isSoftwareImageFeaturePrompt(prompt string) bool {
	softwareTerms := []string{
		"fitur", "feature", "implement", "implementation", "kode", "code", "coding", "endpoint",
		"component", "komponen", "ui", "aplikasi", "app", "tool", "tools", "workflow",
		"upload", "api", "backend", "frontend", "integrasi", "integration", "plugin", "sdk",
	}
	for _, term := range softwareTerms {
		if strings.Contains(prompt, term) {
			return true
		}
	}
	imageToImageTerms := []string{"image to image", "image-to-image", "img2img", "edit image", "image edit", "edit gambar"}
	for _, term := range imageToImageTerms {
		if strings.Contains(prompt, term) {
			return true
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

// buildFallbackSummary composes a useful reply when the supervisor's final
// response is empty (typically because the model emitted only DSML tool
// calls that got stripped, or hit the max-iteration cap). Returning an
// empty string in Telegram looks like the bot froze; this function lets
// the user see the intermediate progress instead.
// FIXED: now calls llm.SanitizeForUser for every intermediate thought to
// guarantee zero DSML leakage (skill_run, parameter tags, etc.)
func buildFallbackSummary(thoughts []string, tools []string) string {
	var sb strings.Builder
	sb.WriteString("⚠ Smara tidak menghasilkan jawaban final yang jelas — tool loop berhenti tanpa kesimpulan.\n\n")

	// Keep the last 3 non-empty thoughts, sanitized aggressively.
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
