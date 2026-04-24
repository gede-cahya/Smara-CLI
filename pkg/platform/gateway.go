package platform

import (
	"context"
	"fmt"
	"log"
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
	adapters    map[string]PlatformAdapter
	supervisor  *agent.Supervisor
	sessions    map[string]*PlatformSession // channelID → session
	auth        *AuthManager
	rateLimiter *RateLimiter
	metrics     *metrics.MetricsCollector
	mu          sync.RWMutex
}

// NewGateway creates a new Gateway with the given supervisor.
func NewGateway(supervisor *agent.Supervisor) *Gateway {
	return &Gateway{
		adapters:    make(map[string]PlatformAdapter),
		supervisor:  supervisor,
		sessions:    make(map[string]*PlatformSession),
		auth:        NewAuthManager(),
		rateLimiter: NewRateLimiter(RateLimitConfig{RequestsPerMinute: 20, BurstSize: 5}),
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
		// Reset conversation history via mode reset
		currentMode := g.supervisor.GetMode()
		g.supervisor.SetMode(currentMode) // SetMode clears history
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
	if ok {
		_ = adapter.SendTyping(ctx, msg.ChannelID)
	}

	// Process via supervisor
	startTime := time.Now()
	result, err := g.supervisor.ProcessPrompt(ctx, msg.Content)
	latencyMs := time.Since(startTime).Milliseconds()

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
		cost := metrics.EstimateCost(
			g.supervisor.GetProviderName(), "",
			int64(result.InputTokens), int64(result.OutputTokens),
		)
		g.metrics.RecordLLMUsage(result.InputTokens, result.OutputTokens, latencyMs, cost)
	}

	return g.sendReply(ctx, msg, result.Response)
}

// sendReply sends a response back to the platform where the message originated.
func (g *Gateway) sendReply(ctx context.Context, original IncomingMessage, content string) error {
	g.mu.RLock()
	adapter, ok := g.adapters[original.Platform]
	g.mu.RUnlock()
	if !ok {
		return fmt.Errorf("adapter tidak ditemukan untuk platform: %s", original.Platform)
	}

	// Split long messages
	parts := splitMessage(content, maxMessageLength)
	for _, part := range parts {
		outMsg := OutgoingMessage{
			Content: part,
			Format:  FormatMarkdown,
			ReplyTo: original.ID,
		}
		if err := adapter.SendMessage(ctx, original.ChannelID, outMsg); err != nil {
			return fmt.Errorf("gagal mengirim reply ke %s: %w", original.Platform, err)
		}
	}

	return nil
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
