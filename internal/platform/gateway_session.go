package platform

import (
	"fmt"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

// platformSessionKey returns a stable conversation key for a platform message.
// Private chats are keyed by user, while groups/channels are keyed by channel so
// every Telegram chat keeps its own context and never inherits CLI/other chat history.
func platformSessionKey(msg IncomingMessage) string {
	platform := strings.TrimSpace(msg.Platform)
	channel := strings.TrimSpace(msg.ChannelID)
	user := strings.TrimSpace(msg.UserID)
	if channel == "" {
		channel = user
	}
	if user == "" {
		user = channel
	}
	return platform + ":" + channel + ":" + user
}

// ensurePlatformSessionLocked binds the incoming platform conversation to a
// dedicated supervisor session, then switches the supervisor to that session.
// Caller must hold g.promptMu because Supervisor keeps a global current history.
func (g *Gateway) ensurePlatformSessionLocked(msg IncomingMessage) error {
	key := platformSessionKey(msg)
	now := time.Now()

	g.mu.RLock()
	ps, ok := g.sessions[key]
	g.mu.RUnlock()

	if ok && ps != nil && ps.SessionID != "" {
		if _, exists := g.supervisor.GetSession(ps.SessionID); exists {
			ps.LastMsg = now
			return g.supervisor.SwitchSession(ps.SessionID)
		}
		// Session was removed/reset; recreate below.
	}

	mode := "ask"
	if ok && ps != nil && ps.Mode != "" {
		mode = ps.Mode
	}
	name := fmt.Sprintf("%s:%s", msg.Platform, msg.ChannelID)
	if msg.Username != "" {
		name = fmt.Sprintf("%s:%s@%s", msg.Platform, msg.Username, msg.ChannelID)
	}

	sess, err := g.supervisor.CreateSession(agent.SessionConfig{
		Name:      name,
		Mode:      mode,
		IsAgentic: true,
	})
	if err != nil {
		return err
	}

	g.mu.Lock()
	g.sessions[key] = &PlatformSession{
		Platform:  msg.Platform,
		ChannelID: msg.ChannelID,
		UserID:    msg.UserID,
		Mode:      mode,
		SessionID: sess.ID,
		CreatedAt: now,
		LastMsg:   now,
	}
	g.mu.Unlock()

	return g.supervisor.SwitchSession(sess.ID)
}
