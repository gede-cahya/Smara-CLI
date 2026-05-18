package discord

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const prdButtonPrefix = "smara_prd"

type prdStep int

const (
	prdStepProductType prdStep = iota
	prdStepTargetUser
	prdStepPlatform
	prdStepScope
	prdStepDetail
	prdStepDone
)

type prdOption struct {
	Label string
	Value string
	Emoji string
}

type prdQuestion struct {
	Title   string
	Options []prdOption
}

type prdWizardSession struct {
	ID        string
	GuildID   string
	ChannelID string
	UserID    string
	Username  string
	MessageID string
	Step      prdStep
	Answers   PRDAnswers
	CreatedAt time.Time
	UpdatedAt time.Time
}

type prdWizardStore struct {
	mu       sync.Mutex
	sessions map[string]*prdWizardSession
}

func newPRDWizardStore() *prdWizardStore {
	return &prdWizardStore{sessions: make(map[string]*prdWizardSession)}
}

func (s *prdWizardStore) start(guildID, channelID, userID, username, idea string) *prdWizardSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	sess := &prdWizardSession{
		ID:        randomSessionID(),
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    userID,
		Username:  username,
		Step:      prdStepProductType,
		CreatedAt: now,
		UpdatedAt: now,
		Answers: PRDAnswers{
			ProductName: inferProductName(idea),
			Idea:        strings.TrimSpace(idea),
			CreatedBy:   username,
			CreatedAt:   now,
		},
	}
	s.sessions[sess.ID] = sess
	return sess
}

func (s *prdWizardStore) get(id string) (*prdWizardSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *prdWizardStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *prdWizardStore) cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, sess := range s.sessions {
		if sess.UpdatedAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

func (s *prdWizardStore) apply(sessionID, userID, value string) (*prdWizardSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, false, fmt.Errorf("session PRD sudah tidak aktif")
	}
	if sess.UserID != userID {
		return sess, false, fmt.Errorf("hanya pembuat wizard yang bisa memilih jawaban")
	}

	label := prdOptionLabel(sess.Step, value)
	if label == "" {
		label = value
	}
	switch sess.Step {
	case prdStepProductType:
		sess.Answers.ProductType = label
		sess.Step = prdStepTargetUser
	case prdStepTargetUser:
		sess.Answers.TargetUser = label
		sess.Step = prdStepPlatform
	case prdStepPlatform:
		sess.Answers.Platform = label
		sess.Step = prdStepScope
	case prdStepScope:
		sess.Answers.Scope = label
		sess.Step = prdStepDetail
	case prdStepDetail:
		sess.Answers.DetailLevel = label
		sess.Step = prdStepDone
	default:
		return sess, true, nil
	}
	sess.UpdatedAt = time.Now()
	return sess, true, nil
}

func prdQuestions(step prdStep) prdQuestion {
	switch step {
	case prdStepProductType:
		return prdQuestion{Title: "Quest 1/5 — Pilih tipe produk", Options: []prdOption{{"SaaS", "saas", "☁️"}, {"Mobile App", "mobile", "📱"}, {"Web App", "webapp", "🌐"}, {"Bot/Automation", "bot", "🤖"}, {"Internal Tool", "internal", "🛠️"}}}
	case prdStepTargetUser:
		return prdQuestion{Title: "Quest 2/5 — Target user utama", Options: []prdOption{{"Consumer", "consumer", "👤"}, {"Business", "business", "🏢"}, {"Developer", "developer", "💻"}, {"Internal Team", "team", "👥"}, {"Community", "community", "🌍"}}}
	case prdStepPlatform:
		return prdQuestion{Title: "Quest 3/5 — Platform utama", Options: []prdOption{{"Web", "web", "🌐"}, {"Mobile", "mobile", "📱"}, {"Desktop", "desktop", "🖥️"}, {"Discord Bot", "discord", "💬"}, {"Multi-platform", "multi", "🔀"}}}
	case prdStepScope:
		return prdQuestion{Title: "Quest 4/5 — Scope awal", Options: []prdOption{{"Prototype", "prototype", "🧪"}, {"MVP", "mvp", "🚀"}, {"V1", "v1", "📦"}, {"Enterprise", "enterprise", "🏛️"}}}
	case prdStepDetail:
		return prdQuestion{Title: "Quest 5/5 — Detail output PRD", Options: []prdOption{{"Ringkas", "brief", "⚡"}, {"Standard", "standard", "📝"}, {"Lengkap", "full", "📚"}}}
	default:
		return prdQuestion{}
	}
}

func prdOptionLabel(step prdStep, value string) string {
	for _, opt := range prdQuestions(step).Options {
		if opt.Value == value {
			return opt.Label
		}
	}
	return ""
}

func renderPRDWizardMessage(sess *prdWizardSession) string {
	q := prdQuestions(sess.Step)
	idea := sess.Answers.Idea
	if idea == "" {
		idea = "Belum diisi — PRD akan memakai placeholder ide produk."
	}
	return fmt.Sprintf("🧩 **Smara PRD Wizard**\n\n%s\n\nIde: `%s`\n\n**Flow chat plain yang akan masuk ke PRD:**\n```text\nUser -> Bot: kirim ide produk\nBot -> User: tanya tipe produk\nUser -> Bot: pilih tipe produk\nBot -> User: tanya target user\nUser -> Bot: pilih target user\nBot -> User: tanya platform\nUser -> Bot: pilih platform\nBot -> User: tanya scope\nUser -> Bot: pilih scope\nBot -> User: tanya detail PRD\nUser -> Bot: pilih detail\nBot -> User: generate Markdown + file .md\n```\n\nKlik salah satu pilihan di bawah. Setelah semua quest selesai, Smara akan mengirim PRD Markdown + file `.md` untuk download/copy-paste.", q.Title, truncateDiscord(idea, 160))
}

func renderPRDComponents(sess *prdWizardSession) []discordgo.MessageComponent {
	q := prdQuestions(sess.Step)
	buttons := make([]discordgo.MessageComponent, 0, len(q.Options))
	for _, opt := range q.Options {
		buttons = append(buttons, discordgo.Button{
			Label:    opt.Label,
			Emoji:    &discordgo.ComponentEmoji{Name: opt.Emoji},
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("%s:%s:%s", prdButtonPrefix, sess.ID, opt.Value),
		})
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}
}

func parsePRDButtonID(customID string) (sessionID, value string, ok bool) {
	parts := strings.Split(customID, ":")
	if len(parts) != 3 || parts[0] != prdButtonPrefix {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func randomSessionID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func truncateDiscord(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
