package platform

import "strings"

type SensitiveDataGuard struct {
	OwnerIDs                 []string
	SensitiveKeywords        []string
	DenyMessage              string
	PromptControlDenyMessage string
}

func (g *Gateway) SetSensitiveDataGuard(platform string, guard SensitiveDataGuard) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sensitiveGuards == nil {
		g.sensitiveGuards = make(map[string]SensitiveDataGuard)
	}
	g.sensitiveGuards[platform] = normalizeSensitiveDataGuard(guard)
}

func (g *Gateway) checkSensitiveDataAccess(msg IncomingMessage) (bool, string) {
	g.mu.RLock()
	guard, ok := g.sensitiveGuards[msg.Platform]
	g.mu.RUnlock()
	if !ok {
		return false, ""
	}
	if guard.isOwner(msg.UserID) {
		return false, ""
	}
	if isOwnerOnlyCommand(msg.Command) || containsPromptControlRequest(msg.Content) {
		if guard.PromptControlDenyMessage != "" {
			return true, guard.PromptControlDenyMessage
		}
		return true, defaultPromptControlDenyMessage
	}
	if !containsSensitiveDataRequest(msg.Content, guard.SensitiveKeywords) {
		return false, ""
	}
	if guard.DenyMessage != "" {
		return true, guard.DenyMessage
	}
	return true, defaultSensitiveDataDenyMessage
}

func (guard SensitiveDataGuard) isOwner(userID string) bool {
	for _, ownerID := range guard.OwnerIDs {
		if userID == ownerID {
			return true
		}
	}
	return false
}

func normalizeSensitiveDataGuard(guard SensitiveDataGuard) SensitiveDataGuard {
	ownerIDs := make([]string, 0, len(guard.OwnerIDs))
	for _, id := range guard.OwnerIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ownerIDs = append(ownerIDs, id)
		}
	}
	keywords := guard.SensitiveKeywords
	if len(keywords) == 0 {
		keywords = defaultSensitiveKeywords
	}
	return SensitiveDataGuard{
		OwnerIDs:                 ownerIDs,
		SensitiveKeywords:        normalizeSensitiveKeywords(keywords),
		DenyMessage:              strings.TrimSpace(guard.DenyMessage),
		PromptControlDenyMessage: strings.TrimSpace(guard.PromptControlDenyMessage),
	}
}

func isOwnerOnlyCommand(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "mode", "mcp", "clear":
		return true
	default:
		return false
	}
}

func containsPromptControlRequest(prompt string) bool {
	normalized := normalizeSensitiveText(prompt)
	if normalized == "" {
		return false
	}
	if containsAnySensitiveKeyword(normalized, defaultPromptControlPhrases) {
		return true
	}
	return containsAnySensitiveKeyword(normalized, promptControlActions) && containsAnySensitiveKeyword(normalized, promptControlTargets)
}

func containsSensitiveDataRequest(prompt string, keywords []string) bool {
	normalized := normalizeSensitiveText(prompt)
	if normalized == "" {
		return false
	}
	return containsAnySensitiveKeyword(normalized, keywords)
}

func containsAnySensitiveKeyword(normalized string, keywords []string) bool {
	padded := " " + normalized + " "
	fields := map[string]bool{}
	for _, field := range strings.Fields(normalized) {
		fields[field] = true
	}
	for _, keyword := range normalizeSensitiveKeywords(keywords) {
		if keyword == "" {
			continue
		}
		if strings.Contains(keyword, " ") {
			if strings.Contains(padded, " "+keyword+" ") {
				return true
			}
			continue
		}
		if fields[keyword] {
			return true
		}
	}
	return false
}

func normalizeSensitiveKeywords(keywords []string) []string {
	out := make([]string, 0, len(keywords))
	seen := map[string]bool{}
	for _, keyword := range keywords {
		keyword = normalizeSensitiveText(keyword)
		if keyword == "" || seen[keyword] {
			continue
		}
		seen[keyword] = true
		out = append(out, keyword)
	}
	return out
}

func normalizeSensitiveText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"_", " ", "-", " ", ".", " ", ":", " ", ";", " ", ",", " ",
		"/", " ", "\\", " ", "\n", " ", "\t", " ", "\r", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ",
		"'", " ", "\"", " ", "`", " ", "=", " ", "|", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}

func redactSensitiveLogContent(content string) string {
	if containsSensitiveDataRequest(content, defaultSensitiveKeywords) {
		return "[redacted sensitive prompt]"
	}
	return content
}

var defaultSensitiveKeywords = []string{
	"password", "passwd", "pwd", "token", "access token", "refresh token",
	"api key", "apikey", "secret", "client secret", "credential", "credentials",
	"kredensial", "private key", "ssh key", "database", "db", "dsn", "connection string",
	"session", "cookie", "otp", "2fa", "nomor hp", "phone", "email", "email user",
	"payment", "pembayaran", "saldo", "transaksi", "rekening", "wallet", "data sensitif",
	"data penting", "rahasia",
}

var promptControlActions = []string{
	"ubah", "ganti", "atur", "setting", "set", "edit", "update", "hapus", "delete", "remove",
	"buat", "buatkan", "create", "tambah", "add", "install", "akses", "access", "pakai", "use",
	"jalankan", "run", "execute", "eksekusi", "disable", "enable", "aktifkan", "nonaktifkan",
}

var promptControlTargets = []string{
	"prompt", "system prompt", "instruksi", "instruction", "skill", "config", "konfigurasi",
	"model", "tools", "tool", "mcp", "admin", "admin command", "owner id", "owner_id",
	"behavior", "behaviour", "perilaku", "bot behavior", "developer message", "system message",
}

var defaultPromptControlPhrases = []string{
	"ubah prompt", "ganti prompt", "edit prompt", "system prompt", "ubah skill", "hapus skill",
	"install skill", "ubah config", "ubah konfigurasi", "ganti model", "akses tools", "akses tool",
	"admin command", "ubah behavior bot", "ubah perilaku bot", "developer message", "system message",
}

const defaultSensitiveDataDenyMessage = "⛔ Akses ditolak. Data sensitif hanya bisa diminta oleh owner terverifikasi."
const defaultPromptControlDenyMessage = "⛔ Akses ditolak. Prompt control dan perintah admin hanya bisa dipakai oleh owner terverifikasi."
