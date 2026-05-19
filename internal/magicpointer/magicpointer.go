package magicpointer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeReadOnly Mode = "read_only"
	ModePlanOnly Mode = "plan_only"
	ModeExecute  Mode = "execute"
)

type Box struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func (b Box) Center() (int, int) { return b.X + b.W/2, b.Y + b.H/2 }

type Element struct {
	Type       string            `json:"type"`
	Text       string            `json:"text"`
	Confidence float64           `json:"confidence"`
	Source     string            `json:"source"`
	Box        *Box              `json:"box,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}
type PlannedAction struct {
	Type                 string   `json:"type"`
	Target               *Element `json:"target,omitempty"`
	Value                string   `json:"value,omitempty"`
	Reason               string   `json:"reason"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
	Risk                 string   `json:"risk"`
}
type ExecutedAction struct {
	Type      string    `json:"type"`
	Target    string    `json:"target,omitempty"`
	X         int       `json:"x,omitempty"`
	Y         int       `json:"y,omitempty"`
	Value     string    `json:"value,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
type ActionPlan struct {
	Instruction string           `json:"instruction"`
	Mode        Mode             `json:"mode"`
	Summary     string           `json:"summary"`
	Actions     []PlannedAction  `json:"actions"`
	Executed    []ExecutedAction `json:"executed,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
}
type AppContext struct {
	AppName     string   `json:"app_name,omitempty"`
	WindowTitle string   `json:"window_title,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	Source      string   `json:"source,omitempty"`
	Hints       []string `json:"hints,omitempty"`
}
type VoiceContext struct {
	Enabled    bool   `json:"enabled"`
	AudioPath  string `json:"audio_path,omitempty"`
	Transcript string `json:"transcript,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Error      string `json:"error,omitempty"`
}

type PrivacyConfig struct {
	PrivacyMode     string   `json:"privacy_mode"`
	BlockedApps     []string `json:"blocked_apps,omitempty"`
	AllowedApps     []string `json:"allowed_apps,omitempty"`
	RedactSensitive bool     `json:"redact_sensitive"`
	LearningEnabled bool     `json:"learning_enabled"`
	AuditEnabled    bool     `json:"audit_enabled"`
}
type PrivacyReport struct {
	ConfigPath string   `json:"config_path,omitempty"`
	Mode       string   `json:"mode"`
	AppBlocked bool     `json:"app_blocked"`
	Reason     string   `json:"reason,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}
type LearningEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	AppProfile  string    `json:"app_profile,omitempty"`
	AppName     string    `json:"app_name,omitempty"`
	Instruction string    `json:"instruction,omitempty"`
	ActionTypes []string  `json:"action_types,omitempty"`
}
type LearningProfile struct {
	UpdatedAt         time.Time       `json:"updated_at"`
	TotalEvents       int             `json:"total_events"`
	AppUsage          map[string]int  `json:"app_usage,omitempty"`
	ActionUsage       map[string]int  `json:"action_usage,omitempty"`
	RecentEvents      []LearningEvent `json:"recent_events,omitempty"`
	SuggestedRoutines []string        `json:"suggested_routines,omitempty"`
}

type ScreenContext struct {
	Timestamp      time.Time        `json:"timestamp"`
	Mode           Mode             `json:"mode"`
	ScreenshotPath string           `json:"screenshot_path,omitempty"`
	ScreenshotHash string           `json:"screenshot_hash,omitempty"`
	OCRAvailable   bool             `json:"ocr_available"`
	RawText        string           `json:"raw_text,omitempty"`
	Summary        string           `json:"summary"`
	Elements       []Element        `json:"elements"`
	App            AppContext       `json:"app,omitempty"`
	Voice          *VoiceContext    `json:"voice,omitempty"`
	Privacy        *PrivacyReport   `json:"privacy,omitempty"`
	Learning       *LearningProfile `json:"learning,omitempty"`
	Plan           *ActionPlan      `json:"plan,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
}

type Options struct {
	OutputDir         string
	ScreenshotPath    string
	KeepScreenshot    bool
	RedactSensitive   bool
	AuditLogPath      string
	Instruction       string
	Execute           bool
	AssumeYes         bool
	Executor          Executor
	Voice             bool
	VoiceFile         string
	VoiceDuration     time.Duration
	AppMode           string
	PrivacyConfigPath string
	LearningEnabled   bool
}
type Executor interface {
	Click(context.Context, int, int) (string, error)
	TypeText(context.Context, string) (string, error)
	Scroll(context.Context, string) (string, error)
	Key(context.Context, string) (string, error)
}

// AppOpener is implemented by executors that can open desktop applications.
type AppOpener interface {
	OpenApp(context.Context, string) (string, error)
}

func Observe(ctx context.Context, opts Options) (ScreenContext, error) {
	if !opts.RedactSensitive {
		opts.RedactSensitive = true
	}
	mode := ModeReadOnly
	instruction := strings.TrimSpace(opts.Instruction)
	out := ScreenContext{Timestamp: time.Now()}
	if opts.Voice || opts.VoiceFile != "" {
		vc := ResolveVoiceInstruction(ctx, opts)
		out.Voice = &vc
		if instruction == "" && vc.Transcript != "" {
			instruction = vc.Transcript
		}
		if vc.Error != "" {
			out.Warnings = append(out.Warnings, "voice: "+vc.Error)
		}
	}
	if instruction != "" {
		mode = ModePlanOnly
	}
	if opts.Execute {
		mode = ModeExecute
	}
	out.Mode = mode
	out.App = DetectAppContext(ctx, opts.AppMode)
	privacy := LoadPrivacyConfig(opts.PrivacyConfigPath)
	preport := EvaluatePrivacy(privacy, out.App, opts.PrivacyConfigPath)
	out.Privacy = &preport
	if privacy.RedactSensitive {
		opts.RedactSensitive = true
	}
	shot := opts.ScreenshotPath
	if shot == "" {
		var err error
		shot, err = CaptureScreenshot(ctx, opts.OutputDir)
		if err != nil {
			out.Warnings = append(out.Warnings, err.Error())
		} else if !opts.KeepScreenshot {
			defer os.Remove(shot)
		}
	}
	out.ScreenshotPath = shot
	if shot != "" {
		if h, err := fileSHA256(shot); err == nil {
			out.ScreenshotHash = h
		}
	}
	if preport.AppBlocked || strings.EqualFold(preport.Mode, "strict") {
		out.Warnings = append(out.Warnings, "privacy mode aktif: OCR/plan/execute diblokir untuk konteks ini")
		out.Summary = "Privacy mode aktif; isi layar tidak dibaca."
		if opts.AuditLogPath != "" && privacy.AuditEnabled {
			_ = AppendAudit(opts.AuditLogPath, out)
		}
		return out, nil
	}
	text, elements, err := OCRDetailed(ctx, shot)
	if err != nil {
		out.Warnings = append(out.Warnings, err.Error())
	} else {
		out.OCRAvailable = true
		if opts.RedactSensitive {
			text = Redact(text)
			for i := range elements {
				elements[i].Text = Redact(elements[i].Text)
			}
		}
		out.RawText = strings.TrimSpace(text)
	}
	if len(elements) == 0 {
		elements = InferElements(out.RawText)
	}
	elements = EnrichVisualElements(elements)
	elements = ApplyAppAwareBoosts(elements, out.App)
	out.Elements = elements
	out.Summary = Summarize(out.RawText, out.Elements, out.OCRAvailable)
	if out.App.Profile != "" {
		out.Summary += " App-aware profile: " + out.App.Profile + "."
	}
	if instruction != "" {
		plan := PlanInstructionWithApp(instruction, out.Elements, out.App)
		if opts.Execute {
			plan.Mode = ModeExecute
			ex, w := ExecutePlan(ctx, plan, ExecuteOptions{AssumeYes: opts.AssumeYes, Executor: opts.Executor})
			plan.Executed = ex
			plan.Warnings = append(plan.Warnings, w...)
		} else {
			plan.Warnings = append(plan.Warnings, "Mode plan-only: tidak melakukan klik/typing/scroll otomatis. Tambahkan --execute.")
		}
		out.Plan = &plan
	}
	if instruction != "" && (opts.LearningEnabled || privacy.LearningEnabled) {
		if lp, err := RecordLearning(defaultLearningPath(), out.App, instruction, out.Plan); err == nil {
			out.Learning = lp
		} else {
			out.Warnings = append(out.Warnings, "learning gagal: "+err.Error())
		}
	}
	if opts.AuditLogPath != "" && privacy.AuditEnabled {
		if err := AppendAudit(opts.AuditLogPath, out); err != nil {
			out.Warnings = append(out.Warnings, "audit log gagal: "+err.Error())
		}
	}
	return out, nil
}

func ResolveVoiceInstruction(ctx context.Context, opts Options) VoiceContext {
	vc := VoiceContext{Enabled: true, AudioPath: opts.VoiceFile}
	audio := opts.VoiceFile
	if audio == "" {
		dur := opts.VoiceDuration
		if dur <= 0 {
			dur = 5 * time.Second
		}
		var err error
		audio, err = RecordVoice(ctx, opts.OutputDir, dur)
		vc.AudioPath = audio
		if err != nil {
			vc.Error = err.Error()
			return vc
		}
	}
	txt, tool, err := TranscribeVoice(ctx, audio)
	vc.Tool = tool
	if err != nil {
		vc.Error = err.Error()
		return vc
	}
	vc.Transcript = strings.TrimSpace(txt)
	return vc
}
func RecordVoice(ctx context.Context, dir string, dur time.Duration) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0755)
	out := filepath.Join(dir, "magic-pointer-voice-"+time.Now().Format("20060102-150405")+".wav")
	sec := strconv.Itoa(maxInt(1, int(dur.Seconds())))
	if _, err := exec.LookPath("arecord"); err == nil {
		return out, exec.CommandContext(ctx, "arecord", "-q", "-d", sec, "-f", "cd", out).Run()
	}
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return out, exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "pulse", "-i", "default", "-t", sec, out).Run()
	}
	return "", errors.New("voice record tidak tersedia: install arecord atau ffmpeg, atau gunakan --voice-file")
}
func TranscribeVoice(ctx context.Context, audio string) (string, string, error) {
	if audio == "" {
		return "", "", errors.New("audio voice kosong")
	}
	if _, err := exec.LookPath("whisper"); err == nil {
		b, e := exec.CommandContext(ctx, "whisper", audio, "--model", "base", "--language", "Indonesian", "--fp16", "False", "--output_format", "txt", "--output_dir", filepath.Dir(audio)).CombinedOutput()
		txtPath := strings.TrimSuffix(audio, filepath.Ext(audio)) + ".txt"
		if data, er := os.ReadFile(txtPath); er == nil && strings.TrimSpace(string(data)) != "" {
			return string(data), "whisper", nil
		}
		if e != nil {
			return "", "whisper", fmt.Errorf("whisper gagal: %s", strings.TrimSpace(string(b)))
		}
	}
	for _, bin := range []string{"whisper-cli", "main"} {
		if _, err := exec.LookPath(bin); err == nil {
			b, e := exec.CommandContext(ctx, bin, "-m", "models/ggml-base.bin", "-f", audio, "-l", "id", "-nt").CombinedOutput()
			if e != nil {
				return "", bin, fmt.Errorf("%s gagal: %s", bin, strings.TrimSpace(string(b)))
			}
			return cleanupWhisperOutput(string(b)), bin, nil
		}
	}
	return "", "", errors.New("STT tidak tersedia: install whisper/openai-whisper atau whisper.cpp (whisper-cli), atau pakai --ask")
}
func cleanupWhisperOutput(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	re := regexp.MustCompile(`\[[0-9:.\s]+-->.*\]`)
	for _, l := range lines {
		l = strings.TrimSpace(re.ReplaceAllString(l, ""))
		if l != "" && !strings.Contains(strings.ToLower(l), "whisper") {
			out = append(out, l)
		}
	}
	return strings.Join(out, " ")
}

func DetectAppContext(ctx context.Context, forced string) AppContext {
	if forced != "" && forced != "auto" {
		return appContextFromName(forced, "manual", "")
	}
	if idb, err := exec.CommandContext(ctx, "xdotool", "getactivewindow").Output(); err == nil {
		id := strings.TrimSpace(string(idb))
		titleb, _ := exec.CommandContext(ctx, "xdotool", "getwindowname", id).Output()
		appb, _ := exec.CommandContext(ctx, "xdotool", "getwindowclassname", id).Output()
		app := strings.TrimSpace(string(appb))
		title := strings.TrimSpace(string(titleb))
		ac := appContextFromName(app, "xdotool", title)
		if ac.AppName == "" {
			ac.AppName = app
		}
		return ac
	}
	if b, err := exec.CommandContext(ctx, "swaymsg", "-t", "get_tree").Output(); err == nil {
		s := string(b)
		re := regexp.MustCompile(`"app_id":\s*"([^"]+)".*?"focused":\s*true`)
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			return appContextFromName(m[1], "swaymsg", "")
		}
	}
	return AppContext{}
}
func appContextFromName(name, source, title string) AppContext {
	low := strings.ToLower(name + " " + title)
	ac := AppContext{AppName: name, WindowTitle: title, Source: source}
	switch {
	case strings.Contains(low, "chrome") || strings.Contains(low, "firefox") || strings.Contains(low, "browser") || strings.Contains(low, "edge"):
		ac.Profile = "browser"
		ac.Hints = []string{"tab", "address bar", "back/forward", "search"}
	case strings.Contains(low, "code") || strings.Contains(low, "vscode") || strings.Contains(low, "cursor"):
		ac.Profile = "code_editor"
		ac.Hints = []string{"command palette", "terminal", "file explorer", "search"}
	case strings.Contains(low, "libreoffice") || strings.Contains(low, "calc") || strings.Contains(low, "excel") || strings.Contains(low, "spreadsheet"):
		ac.Profile = "spreadsheet"
		ac.Hints = []string{"cell", "formula", "sheet", "filter"}
	case strings.Contains(low, "thunderbird") || strings.Contains(low, "mail") || strings.Contains(low, "gmail") || strings.Contains(low, "outlook"):
		ac.Profile = "email"
		ac.Hints = []string{"compose", "reply", "send", "archive", "attachment"}
	case strings.Contains(low, "nautilus") || strings.Contains(low, "dolphin") || strings.Contains(low, "files") || strings.Contains(low, "explorer"):
		ac.Profile = "file_manager"
		ac.Hints = []string{"folder", "file", "rename", "copy", "move"}
	case strings.Contains(low, "figma") || strings.Contains(low, "gimp") || strings.Contains(low, "inkscape") || strings.Contains(low, "photoshop"):
		ac.Profile = "design"
		ac.Hints = []string{"layer", "canvas", "tool", "export"}
	}
	return ac
}
func ApplyAppAwareBoosts(elements []Element, app AppContext) []Element {
	if app.Profile == "" {
		return elements
	}
	out := append([]Element{}, elements...)
	kws := profileKeywords(app.Profile)
	for i := range out {
		low := strings.ToLower(out[i].Text)
		for _, kw := range kws {
			if strings.Contains(low, kw) {
				out[i].Confidence += .12
				if out[i].Attributes == nil {
					out[i].Attributes = map[string]string{}
				}
				out[i].Attributes["app_profile_boost"] = app.Profile
				break
			}
		}
	}
	sortElements(out)
	return trimElements(out)
}
func profileKeywords(profile string) []string {
	switch profile {
	case "browser":
		return []string{"tab", "address", "search", "back", "reload", "url", "bookmark"}
	case "code_editor":
		return []string{"terminal", "explorer", "search", "commit", "run", "debug", "command"}
	case "email":
		return []string{"compose", "reply", "send", "archive", "attachment", "inbox"}
	case "spreadsheet":
		return []string{"formula", "cell", "sheet", "filter", "sort"}
	case "file_manager":
		return []string{"folder", "file", "copy", "move", "rename", "delete"}
	case "design":
		return []string{"layer", "canvas", "export", "frame", "tool"}
	default:
		return nil
	}
}
func PlanInstructionWithApp(inst string, elements []Element, app AppContext) ActionPlan {
	if p, ok := PlanDesktopLaunchOrBrowserTask(inst); ok {
		return p
	}
	norm := NormalizeInstructionForApp(inst, app)
	plan := PlanInstruction(norm, elements)
	if app.Profile != "" {
		plan.Summary += " Profile app-aware: " + app.Profile + "."
		if norm != inst {
			plan.Warnings = append(plan.Warnings, "Instruksi dinormalisasi untuk app profile "+app.Profile+": "+norm)
		}
	}
	return plan
}
func NormalizeInstructionForApp(inst string, app AppContext) string {
	low := strings.ToLower(inst)
	switch app.Profile {
	case "browser":
		if strings.Contains(low, "alamat") || strings.Contains(low, "url") {
			return "klik address bar"
		}
		if strings.Contains(low, "tab baru") {
			return "klik new tab"
		}
	case "code_editor":
		if strings.Contains(low, "command palette") || strings.Contains(low, "palet perintah") {
			return "ketik dengan ctrl+shift+p"
		}
		if strings.Contains(low, "terminal") {
			return "klik terminal"
		}
	case "email":
		if strings.Contains(low, "balas") {
			return "klik reply"
		}
		if strings.Contains(low, "lampiran") {
			return "klik attachment"
		}
	case "file_manager":
		if strings.Contains(low, "folder") {
			return strings.ReplaceAll(inst, "direktori", "folder")
		}
	}
	return inst
}

func defaultPrivacyPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return filepath.Join(os.TempDir(), "smara-magic-pointer-privacy.json")
	}
	return filepath.Join(home, ".smara", "magic-pointer", "privacy.json")
}
func defaultLearningPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return filepath.Join(os.TempDir(), "smara-magic-pointer-learning.json")
	}
	return filepath.Join(home, ".smara", "magic-pointer", "learning.json")
}
func DefaultPrivacyConfig() PrivacyConfig {
	return PrivacyConfig{PrivacyMode: "normal", RedactSensitive: true, LearningEnabled: false, AuditEnabled: true, BlockedApps: []string{"keepass", "1password", "bitwarden", "authenticator", "password"}}
}
func LoadPrivacyConfig(path string) PrivacyConfig {
	if path == "" {
		path = defaultPrivacyPath()
	}
	cfg := DefaultPrivacyConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	if cfg.PrivacyMode == "" {
		cfg.PrivacyMode = "normal"
	}
	return cfg
}
func SavePrivacyConfig(path string, cfg PrivacyConfig) error {
	if path == "" {
		path = defaultPrivacyPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, b, 0600)
}
func EvaluatePrivacy(cfg PrivacyConfig, app AppContext, path string) PrivacyReport {
	if path == "" {
		path = defaultPrivacyPath()
	}
	r := PrivacyReport{ConfigPath: path, Mode: cfg.PrivacyMode}
	name := strings.ToLower(app.AppName + " " + app.WindowTitle + " " + app.Profile)
	for _, b := range cfg.BlockedApps {
		if b != "" && strings.Contains(name, strings.ToLower(b)) {
			r.AppBlocked = true
			r.Reason = "app cocok blocklist: " + b
			return r
		}
	}
	if strings.EqualFold(cfg.PrivacyMode, "allowlist") {
		ok := false
		for _, a := range cfg.AllowedApps {
			if a != "" && strings.Contains(name, strings.ToLower(a)) {
				ok = true
				break
			}
		}
		if !ok {
			r.AppBlocked = true
			r.Reason = "privacy allowlist aktif dan app belum diizinkan"
		}
	}
	return r
}
func UpdatePrivacyConfig(path, mode, blockApp, allowApp string, learning *bool) (PrivacyConfig, error) {
	cfg := LoadPrivacyConfig(path)
	if mode != "" {
		cfg.PrivacyMode = mode
	}
	if blockApp != "" {
		cfg.BlockedApps = appendUnique(cfg.BlockedApps, blockApp)
	}
	if allowApp != "" {
		cfg.AllowedApps = appendUnique(cfg.AllowedApps, allowApp)
	}
	if learning != nil {
		cfg.LearningEnabled = *learning
	}
	return cfg, SavePrivacyConfig(path, cfg)
}
func appendUnique(xs []string, v string) []string {
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			return xs
		}
	}
	return append(xs, v)
}
func LoadLearningProfile(path string) (*LearningProfile, error) {
	if path == "" {
		path = defaultLearningPath()
	}
	lp := &LearningProfile{AppUsage: map[string]int{}, ActionUsage: map[string]int{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return lp, nil
	}
	if err := json.Unmarshal(b, lp); err != nil {
		return nil, err
	}
	if lp.AppUsage == nil {
		lp.AppUsage = map[string]int{}
	}
	if lp.ActionUsage == nil {
		lp.ActionUsage = map[string]int{}
	}
	return lp, nil
}
func RecordLearning(path string, app AppContext, instruction string, plan *ActionPlan) (*LearningProfile, error) {
	lp, err := LoadLearningProfile(path)
	if err != nil {
		return nil, err
	}
	e := LearningEvent{Timestamp: time.Now(), AppProfile: app.Profile, AppName: app.AppName, Instruction: Redact(instruction)}
	if plan != nil {
		for _, a := range plan.Actions {
			e.ActionTypes = append(e.ActionTypes, a.Type)
			lp.ActionUsage[a.Type]++
		}
	}
	key := app.Profile
	if key == "" {
		key = app.AppName
	}
	if key == "" {
		key = "unknown"
	}
	lp.AppUsage[key]++
	lp.TotalEvents++
	lp.UpdatedAt = time.Now()
	lp.RecentEvents = append([]LearningEvent{e}, lp.RecentEvents...)
	if len(lp.RecentEvents) > 20 {
		lp.RecentEvents = lp.RecentEvents[:20]
	}
	lp.SuggestedRoutines = suggestRoutines(lp)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(lp, "", "  ")
	return lp, os.WriteFile(path, b, 0600)
}
func suggestRoutines(lp *LearningProfile) []string {
	var out []string
	for app, n := range lp.AppUsage {
		if n >= 3 {
			out = append(out, fmt.Sprintf("Buat routine untuk app %s (%d kali dipakai)", app, n))
		}
	}
	for act, n := range lp.ActionUsage {
		if n >= 5 {
			out = append(out, fmt.Sprintf("Optimalkan aksi %s yang sering dipakai (%d kali)", act, n))
		}
	}
	sort.Strings(out)
	if len(out) > 5 {
		return out[:5]
	}
	return out
}

func CaptureScreenshot(ctx context.Context, outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = os.TempDir()
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}
	out := filepath.Join(outputDir, "magic-pointer-"+time.Now().Format("20060102-150405")+".png")
	candidates := [][]string{{"grim", out}, {"gnome-screenshot", "-f", out}, {"scrot", out}, {"import", "-window", "root", out}}
	var tried []string
	for _, args := range candidates {
		bin := args[0]
		if _, err := exec.LookPath(bin); err != nil {
			tried = append(tried, bin+"(missing)")
			continue
		}
		cmd := exec.CommandContext(ctx, bin, args[1:]...)
		if b, err := cmd.CombinedOutput(); err == nil {
			return out, nil
		} else {
			tried = append(tried, fmt.Sprintf("%s(%s)", bin, strings.TrimSpace(string(b))))
		}
	}
	return "", fmt.Errorf("tidak bisa mengambil screenshot; install salah satu: grim, gnome-screenshot, scrot, imagemagick/import; tried=%s", strings.Join(tried, ", "))
}
func OCR(ctx context.Context, imagePath string) (string, error) {
	t, _, e := OCRDetailed(ctx, imagePath)
	return t, e
}
func OCRDetailed(ctx context.Context, imagePath string) (string, []Element, error) {
	if imagePath == "" {
		return "", nil, errors.New("OCR dilewati: screenshot tidak tersedia")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", nil, errors.New("OCR tidak tersedia: tesseract belum terinstall")
	}
	for _, lang := range []string{"eng+ind", "eng"} {
		b, err := exec.CommandContext(ctx, "tesseract", imagePath, "stdout", "-l", lang, "tsv").Output()
		if err == nil {
			text, els := parseTesseractTSV(string(b))
			if strings.TrimSpace(text) != "" {
				return text, els, nil
			}
		}
	}
	b, err := exec.CommandContext(ctx, "tesseract", imagePath, "stdout", "-l", "eng").Output()
	if err == nil {
		return string(b), InferElements(string(b)), nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return "", nil, fmt.Errorf("OCR gagal: %s", strings.TrimSpace(string(ee.Stderr)))
	}
	return "", nil, fmt.Errorf("OCR gagal: %w", err)
}

var sensitivePatterns = []*regexp.Regexp{regexp.MustCompile(`(?i)(password|passwd|token|api[_ -]?key|secret)\s*[:=]\s*\S+`), regexp.MustCompile(`\b[\w._%+\-]+@[\w.\-]+\.[A-Za-z]{2,}\b`)}

func Redact(s string) string {
	for _, re := range sensitivePatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

var buttonWords = regexp.MustCompile(`(?i)\b(login|log in|sign in|submit|save|cancel|ok|next|continue|back|send|search|delete|edit|add|create|upload|download|open|close|daftar|masuk|simpan|kirim|lanjut|batal|hapus|ubah|buat|unggah|unduh|buka|tutup|cari|apply|yes|no|confirm|settings|pengaturan|reply|compose|terminal|attachment|address bar|new tab)\b`)
var iconWords = regexp.MustCompile(`(?i)\b(gear|setting|settings|pengaturan|camera|kamera|search|cari|home|menu|profile|profil|trash|delete|hapus|plus|add|tambah|edit|pencil|download|unduh|upload|unggah)\b`)
var checkboxWords = regexp.MustCompile(`(?i)\b(checkbox|centang|ceklist|checklist|check box|radio|opsi|option|dropdown|select|pilih)\b`)

func InferElements(text string) []Element {
	var els []Element
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.Join(strings.Fields(line), " "))
		if line == "" || seen[strings.ToLower(line)] {
			continue
		}
		seen[strings.ToLower(line)] = true
		typ, conf := classifyLine(line)
		els = append(els, Element{Type: typ, Text: line, Confidence: conf, Source: "ocr"})
	}
	sortElements(els)
	return trimElements(els)
}
func classifyLine(line string) (string, float64) {
	low := strings.ToLower(line)
	if checkboxWords.MatchString(line) {
		if strings.Contains(low, "dropdown") || strings.Contains(low, "select") || strings.Contains(low, "pilih") {
			return "dropdown_candidate", .72
		}
		return "checkbox_or_radio_candidate", .72
	}
	if iconWords.MatchString(line) && len([]rune(line)) <= 48 {
		return "icon_candidate", .70
	}
	if buttonWords.MatchString(line) && len([]rune(line)) <= 64 {
		return "action_candidate", .78
	}
	if strings.Contains(low, "http://") || strings.Contains(low, "https://") || strings.Contains(low, "www.") {
		return "link", .72
	}
	if strings.HasSuffix(line, ":") || strings.Contains(low, "search") || strings.Contains(low, "cari") {
		return "input_or_label", .66
	}
	return "text", .62
}
func parseTesseractTSV(tsv string) (string, []Element) {
	type acc struct {
		text                     []string
		left, top, right, bottom int
		confSum                  float64
		count                    int
	}
	groups := map[string]*acc{}
	var order []string
	for i, row := range strings.Split(tsv, "\n") {
		if i == 0 || strings.TrimSpace(row) == "" {
			continue
		}
		cols := strings.Split(row, "\t")
		if len(cols) < 12 || cols[0] != "5" || strings.TrimSpace(cols[11]) == "" {
			continue
		}
		key := strings.Join(cols[1:5], ":")
		l, _ := strconv.Atoi(cols[6])
		t, _ := strconv.Atoi(cols[7])
		w, _ := strconv.Atoi(cols[8])
		h, _ := strconv.Atoi(cols[9])
		c, _ := strconv.ParseFloat(cols[10], 64)
		g := groups[key]
		if g == nil {
			g = &acc{left: l, top: t, right: l + w, bottom: t + h}
			groups[key] = g
			order = append(order, key)
		}
		g.text = append(g.text, cols[11])
		if l < g.left {
			g.left = l
		}
		if t < g.top {
			g.top = t
		}
		if l+w > g.right {
			g.right = l + w
		}
		if t+h > g.bottom {
			g.bottom = t + h
		}
		if c >= 0 {
			g.confSum += c
			g.count++
		}
	}
	var lines []string
	var els []Element
	for _, key := range order {
		g := groups[key]
		line := strings.TrimSpace(strings.Join(g.text, " "))
		if line == "" {
			continue
		}
		lines = append(lines, line)
		typ, base := classifyLine(line)
		conf := base
		if g.count > 0 {
			conf = (g.confSum / float64(g.count)) / 100
			if conf < base {
				conf = (conf + base) / 2
			}
		}
		els = append(els, Element{Type: typ, Text: line, Confidence: conf, Source: "ocr_tsv", Box: &Box{X: g.left, Y: g.top, W: g.right - g.left, H: g.bottom - g.top}})
	}
	sortElements(els)
	return strings.Join(lines, "\n"), trimElements(els)
}
func EnrichVisualElements(elements []Element) []Element {
	if len(elements) == 0 {
		return elements
	}
	out := append([]Element{}, elements...)
	for _, e := range elements {
		if e.Box == nil {
			continue
		}
		low := strings.ToLower(e.Text)
		attrs := map[string]string{"derived_from": e.Type}
		if iconWords.MatchString(low) && e.Type != "icon_candidate" {
			out = append(out, Element{Type: "icon_candidate", Text: e.Text, Confidence: e.Confidence * .9, Source: "visual_lite", Box: expandBox(*e.Box, 8), Attributes: attrs})
		}
		if strings.Contains(low, "☐") || strings.Contains(low, "☑") || strings.Contains(low, "○") || strings.Contains(low, "◉") || checkboxWords.MatchString(low) {
			state := "unknown"
			if strings.Contains(low, "☑") || strings.Contains(low, "◉") {
				state = "checked"
			} else if strings.Contains(low, "☐") || strings.Contains(low, "○") {
				state = "unchecked"
			}
			out = append(out, Element{Type: "checkbox_or_radio_candidate", Text: e.Text, Confidence: e.Confidence * .92, Source: "visual_lite", Box: expandBox(*e.Box, 10), Attributes: map[string]string{"state": state}})
		}
		if strings.Contains(low, "▼") || strings.Contains(low, "▾") || strings.Contains(low, "dropdown") || strings.Contains(low, "select") {
			out = append(out, Element{Type: "dropdown_candidate", Text: e.Text, Confidence: e.Confidence * .92, Source: "visual_lite", Box: expandBox(*e.Box, 10), Attributes: attrs})
		}
		if e.Type == "input_or_label" && e.Box.W > 0 {
			b := Box{X: e.Box.X + e.Box.W + 8, Y: e.Box.Y - 4, W: maxInt(120, e.Box.W*2), H: maxInt(24, e.Box.H+8)}
			out = append(out, Element{Type: "input_field_candidate", Text: e.Text, Confidence: e.Confidence * .80, Source: "layout_lite", Box: &b, Attributes: map[string]string{"derived_from": "label_right"}})
		}
	}
	sortElements(out)
	return trimElements(out)
}
func expandBox(b Box, pad int) *Box {
	if b.X-pad > 0 {
		b.X -= pad
	} else {
		b.X = 0
	}
	if b.Y-pad > 0 {
		b.Y -= pad
	} else {
		b.Y = 0
	}
	b.W += pad * 2
	b.H += pad * 2
	return &b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func PlanInstruction(instruction string, elements []Element) ActionPlan {
	inst := strings.TrimSpace(instruction)
	steps := splitWorkflowInstruction(inst)
	if len(steps) > 1 {
		plan := ActionPlan{Instruction: inst, Mode: ModePlanOnly}
		for _, s := range steps {
			sub := PlanInstruction(s, elements)
			plan.Actions = append(plan.Actions, sub.Actions...)
			plan.Warnings = append(plan.Warnings, sub.Warnings...)
		}
		plan.Summary = fmt.Sprintf("Phase 5 workflow: membuat %d aksi dari %d langkah instruksi.", len(plan.Actions), len(steps))
		return plan
	}
	low := strings.ToLower(inst)
	plan := ActionPlan{Instruction: inst, Mode: ModePlanOnly}
	critical := regexp.MustCompile(`(?i)\b(delete|hapus|send|kirim|pay|bayar|purchase|beli|submit|logout|keluar|close|tutup)\b`).MatchString(inst)
	switch {
	case strings.Contains(low, "scroll") || strings.Contains(low, "gulir"):
		dir := "down"
		if strings.Contains(low, "up") || strings.Contains(low, "atas") {
			dir = "up"
		}
		plan.Actions = append(plan.Actions, PlannedAction{Type: "scroll", Value: dir, Reason: "Instruksi meminta navigasi scroll/gulir.", Risk: "low"})
	case strings.Contains(low, "buka ") || strings.HasPrefix(low, "open "):
		app := extractAppName(inst)
		if app != "" {
			plan.Actions = append(plan.Actions, PlannedAction{Type: "open_app", Value: app, Reason: "Instruksi meminta membuka aplikasi desktop.", RequiresConfirmation: false, Risk: "low"})
		} else {
			target := bestElementMatch(inst, elements, []string{"action_candidate", "icon_candidate", "link", "text"})
			plan.Actions = append(plan.Actions, PlannedAction{Type: "click", Target: target, Reason: "Instruksi buka dipetakan ke target layar.", RequiresConfirmation: critical, Risk: riskLabel(critical)})
		}
	case strings.Contains(low, "ctrl+") || strings.Contains(low, "alt+") || strings.Contains(low, "shift+"):
		plan.Actions = append(plan.Actions, PlannedAction{Type: "key", Value: extractKeyCombo(inst), Reason: "Instruksi/app-aware meminta shortcut keyboard.", RequiresConfirmation: critical, Risk: riskLabel(critical)})
	case strings.Contains(low, "ketik") || strings.Contains(low, "tulis") || strings.Contains(low, "isi"):
		target := bestElementMatch(inst, elements, []string{"input_field_candidate", "input_or_label", "text"})
		if target != nil && target.Type != "input_field_candidate" {
			if f := nearestDerivedInput(*target, elements); f != nil {
				target = f
			}
		}
		if target != nil && target.Box != nil {
			plan.Actions = append(plan.Actions, PlannedAction{Type: "click", Target: target, Reason: "Fokus ke field sebelum mengetik.", Risk: "low"})
		}
		plan.Actions = append(plan.Actions, PlannedAction{Type: "type", Target: target, Value: extractTypedValue(inst), Reason: "Instruksi mengarah ke pengisian/pengetikan teks.", RequiresConfirmation: true, Risk: "medium"})
	case strings.Contains(low, "centang") || strings.Contains(low, "check") || strings.Contains(low, "ceklist"):
		target := bestElementMatch(inst, elements, []string{"checkbox_or_radio_candidate", "text"})
		plan.Actions = append(plan.Actions, PlannedAction{Type: "click", Target: target, Reason: "Instruksi meminta memilih checkbox/radio.", RequiresConfirmation: critical, Risk: riskLabel(critical)})
	case strings.Contains(low, "dropdown") || strings.Contains(low, "pilih") || strings.Contains(low, "select"):
		target := bestElementMatch(inst, elements, []string{"dropdown_candidate", "input_or_label", "text"})
		plan.Actions = append(plan.Actions, PlannedAction{Type: "click", Target: target, Reason: "Instruksi meminta membuka/memilih dropdown atau opsi.", RequiresConfirmation: critical, Risk: riskLabel(critical)})
	default:
		target := bestElementMatch(inst, elements, []string{"action_candidate", "icon_candidate", "link", "input_or_label", "text"})
		plan.Actions = append(plan.Actions, PlannedAction{Type: "click", Target: target, Reason: "Instruksi dipetakan ke target layar paling relevan.", RequiresConfirmation: critical, Risk: riskLabel(critical)})
	}
	if len(elements) == 0 {
		plan.Warnings = append(plan.Warnings, "Tidak ada elemen layar terdeteksi; rencana mungkin tidak akurat.")
	}
	for _, a := range plan.Actions {
		if a.Target == nil && a.Type != "scroll" && a.Type != "type" && a.Type != "key" {
			plan.Warnings = append(plan.Warnings, "Target spesifik belum ditemukan dari OCR/visual-lite.")
			break
		}
	}
	plan.Summary = fmt.Sprintf("Membuat %d aksi terencana dari instruksi pengguna.", len(plan.Actions))
	return plan
}
func splitWorkflowInstruction(inst string) []string {
	re := regexp.MustCompile(`(?i)\s*(?:,?\s+lalu\s+|,?\s+kemudian\s+|\s+dan\s+|\s*;\s*|\s*->\s*)`)
	parts := re.Split(inst, -1)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func nearestDerivedInput(target Element, elements []Element) *Element {
	for _, e := range elements {
		if e.Type == "input_field_candidate" && e.Attributes["derived_from"] == "label_right" && strings.EqualFold(e.Text, target.Text) {
			return &e
		}
	}
	return nil
}
func bestElementMatch(inst string, elements []Element, preferred []string) *Element {
	terms := tokenize(inst)
	pref := map[string]bool{}
	for _, p := range preferred {
		pref[p] = true
	}
	type scored struct {
		e     Element
		score float64
	}
	var scores []scored
	for _, e := range elements {
		s := e.Confidence
		if pref[e.Type] {
			s += .25
		}
		et := strings.ToLower(e.Text)
		for _, term := range terms {
			if len(term) >= 3 && strings.Contains(et, term) {
				s += .35
			}
		}
		if e.Box != nil {
			s += .05
		}
		scores = append(scores, scored{e, s})
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	if len(scores) == 0 {
		return nil
	}
	return &scores[0].e
}
func tokenize(s string) []string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	parts := strings.Fields(re.ReplaceAllString(s, " "))
	stop := map[string]bool{"klik": true, "click": true, "buka": true, "open": true, "tombol": true, "button": true, "menu": true, "yang": true, "ini": true, "ke": true, "di": true, "isi": true, "ketik": true, "tulis": true, "dengan": true, "lalu": true, "kemudian": true, "dan": true, "pilih": true, "select": true, "centang": true}
	var out []string
	for _, p := range parts {
		if !stop[p] {
			out = append(out, p)
		}
	}
	return out
}
func extractTypedValue(inst string) string {
	for _, sep := range []string{" dengan ", " value ", ":"} {
		idx := strings.LastIndex(strings.ToLower(inst), sep)
		if idx >= 0 {
			return strings.TrimSpace(inst[idx+len(sep):])
		}
	}
	return ""
}
func extractKeyCombo(inst string) string {
	re := regexp.MustCompile(`(?i)(ctrl|alt|shift|super)(?:\+(?:ctrl|alt|shift|super|[a-z0-9]))+`)
	if m := re.FindString(inst); m != "" {
		return strings.ReplaceAll(strings.ToLower(m), "+", "+")
	}
	return inst
}

type ExecuteOptions struct {
	AssumeYes bool
	Executor  Executor
}

func ExecutePlan(ctx context.Context, plan ActionPlan, opts ExecuteOptions) ([]ExecutedAction, []string) {
	executor := opts.Executor
	if executor == nil {
		executor = DesktopExecutor{}
	}
	var out []ExecutedAction
	var warnings []string
	for _, a := range plan.Actions {
		rec := ExecutedAction{Type: a.Type, Value: safeAuditValue(a), Timestamp: time.Now()}
		if a.Target != nil {
			rec.Target = a.Target.Text
		}
		if a.RequiresConfirmation && !opts.AssumeYes {
			rec.Error = "aksi butuh konfirmasi; jalankan ulang dengan --yes jika yakin"
			warnings = append(warnings, rec.Error)
			out = append(out, rec)
			break
		}
		switch a.Type {
		case "click":
			if a.Target == nil || a.Target.Box == nil {
				rec.Error = "target click tidak punya bounding box"
			} else {
				rec.X, rec.Y = a.Target.Box.Center()
				rec.Tool, rec.Error = runExecErr(executor.Click(ctx, rec.X, rec.Y))
			}
		case "type":
			if strings.TrimSpace(a.Value) == "" {
				rec.Error = "value type kosong"
			} else {
				rec.Tool, rec.Error = runExecErr(executor.TypeText(ctx, a.Value))
			}
		case "scroll":
			rec.Tool, rec.Error = runExecErr(executor.Scroll(ctx, a.Value))
		case "key":
			rec.Tool, rec.Error = runExecErr(executor.Key(ctx, a.Value))
		case "open_app":
			if strings.TrimSpace(a.Value) == "" {
				rec.Error = "nama aplikasi kosong"
			} else if opener, ok := executor.(AppOpener); ok {
				rec.Tool, rec.Error = runExecErr(opener.OpenApp(ctx, a.Value))
			} else {
				rec.Error = "executor tidak mendukung open_app"
			}
		default:
			rec.Error = "tipe aksi belum didukung: " + a.Type
		}
		rec.Success = rec.Error == ""
		if rec.Error != "" {
			warnings = append(warnings, rec.Error)
			out = append(out, rec)
			break
		}
		out = append(out, rec)
	}
	return out, warnings
}
func runExecErr(tool string, err error) (string, string) {
	if err != nil {
		return tool, err.Error()
	}
	return tool, ""
}
func safeAuditValue(a PlannedAction) string {
	if a.Type == "type" && a.Value != "" {
		return "[TYPED_TEXT_REDACTED]"
	}
	return a.Value
}

type DesktopExecutor struct{}

func (DesktopExecutor) Click(ctx context.Context, x, y int) (string, error) {
	if _, err := exec.LookPath("xdotool"); err == nil {
		return "xdotool", exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y), "click", "1").Run()
	}
	if _, err := exec.LookPath("ydotool"); err == nil {
		return "ydotool", exec.CommandContext(ctx, "ydotool", "mousemove", "-a", strconv.Itoa(x), strconv.Itoa(y), "click", "0xC0").Run()
	}
	return "", errors.New("executor tidak tersedia: install xdotool atau ydotool")
}
func (DesktopExecutor) TypeText(ctx context.Context, text string) (string, error) {
	if _, err := exec.LookPath("xdotool"); err == nil {
		return "xdotool", exec.CommandContext(ctx, "xdotool", "type", "--delay", "5", text).Run()
	}
	if _, err := exec.LookPath("ydotool"); err == nil {
		return "ydotool", exec.CommandContext(ctx, "ydotool", "type", text).Run()
	}
	return "", errors.New("executor tidak tersedia: install xdotool atau ydotool")
}
func (DesktopExecutor) Scroll(ctx context.Context, direction string) (string, error) {
	button := "5"
	if direction == "up" {
		button = "4"
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		return "xdotool", exec.CommandContext(ctx, "xdotool", "click", button).Run()
	}
	if _, err := exec.LookPath("ydotool"); err == nil {
		return "ydotool", exec.CommandContext(ctx, "ydotool", "click", button).Run()
	}
	return "", errors.New("executor tidak tersedia: install xdotool atau ydotool")
}
func (DesktopExecutor) Key(ctx context.Context, key string) (string, error) {
	if _, err := exec.LookPath("xdotool"); err == nil {
		return "xdotool", exec.CommandContext(ctx, "xdotool", "key", key).Run()
	}
	if _, err := exec.LookPath("ydotool"); err == nil {
		return "ydotool", exec.CommandContext(ctx, "ydotool", "key", key).Run()
	}
	return "", errors.New("executor tidak tersedia: install xdotool atau ydotool")
}
func riskLabel(critical bool) string {
	if critical {
		return "medium"
	}
	return "low"
}
func sortElements(els []Element) {
	sort.SliceStable(els, func(i, j int) bool { return els[i].Confidence > els[j].Confidence })
}
func trimElements(els []Element) []Element {
	if len(els) > 50 {
		return els[:50]
	}
	return els
}
func Summarize(text string, els []Element, ocr bool) string {
	if !ocr {
		return "Screen context read-only aktif, tetapi OCR belum tersedia. Screenshot hash tetap dicatat untuk audit."
	}
	counts := map[string]int{}
	for _, e := range els {
		counts[e.Type]++
	}
	return fmt.Sprintf("Terdeteksi sekitar %d kata, %d tombol/aksi, %d ikon, %d checkbox/radio, %d dropdown, dan %d field input.", len(strings.Fields(text)), counts["action_candidate"], counts["icon_candidate"], counts["checkbox_or_radio_candidate"], counts["dropdown_candidate"], counts["input_field_candidate"])
}
func AppendAudit(path string, sc ScreenContext) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(sc)
	_, err = f.Write(append(b, '\n'))
	return err
}
func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// AutopilotOptions controls the observe-plan-execute-recover loop used by Magic Pointer Phase 3.
type AutopilotOptions struct {
	Options
	MaxSteps      int
	StopCondition string
	Observer      func(context.Context, Options) (ScreenContext, error)
}

type AutopilotRun struct {
	Instruction string          `json:"instruction"`
	MaxSteps    int             `json:"max_steps"`
	Completed   bool            `json:"completed"`
	StopReason  string          `json:"stop_reason"`
	Iterations  []ScreenContext `json:"iterations"`
	Warnings    []string        `json:"warnings,omitempty"`
}

// RunAutopilot repeatedly observes the desktop, executes the next safe plan, and stops safely on success/no-target/error/max-steps.
func RunAutopilot(ctx context.Context, opts AutopilotOptions) (AutopilotRun, error) {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 10
	}
	if opts.Observer == nil {
		opts.Observer = Observe
	}
	run := AutopilotRun{Instruction: opts.Instruction, MaxSteps: opts.MaxSteps}
	instruction := strings.TrimSpace(opts.Instruction)
	if instruction == "" {
		run.StopReason = "instruksi kosong"
		return run, nil
	}
	for i := 0; i < opts.MaxSteps; i++ {
		stepOpts := opts.Options
		stepOpts.Instruction = instruction
		stepOpts.Execute = true
		stepOpts.AssumeYes = opts.AssumeYes
		sc, err := opts.Observer(ctx, stepOpts)
		if err != nil {
			run.StopReason = "observe error: " + err.Error()
			return run, err
		}
		run.Iterations = append(run.Iterations, sc)
		if sc.Plan == nil || len(sc.Plan.Actions) == 0 {
			run.StopReason = "tidak ada aksi terencana"
			return run, nil
		}
		if hasMissingTarget(*sc.Plan) {
			run.StopReason = "target UI tidak ditemukan; autopilot berhenti aman"
			run.Warnings = append(run.Warnings, sc.Plan.Warnings...)
			return run, nil
		}
		if len(sc.Plan.Executed) == 0 {
			run.StopReason = "tidak ada aksi dieksekusi"
			return run, nil
		}
		last := sc.Plan.Executed[len(sc.Plan.Executed)-1]
		if !last.Success {
			run.StopReason = "aksi gagal: " + last.Error
			run.Warnings = append(run.Warnings, sc.Plan.Warnings...)
			return run, nil
		}
		if stopConditionMet(opts.StopCondition, sc) || len(sc.Plan.Executed) >= len(sc.Plan.Actions) {
			run.Completed = true
			run.StopReason = "task selesai"
			return run, nil
		}
	}
	run.StopReason = "mencapai max steps"
	return run, nil
}

func hasMissingTarget(plan ActionPlan) bool {
	for _, a := range plan.Actions {
		if a.Type == "click" && (a.Target == nil || a.Target.Box == nil) {
			return true
		}
	}
	return false
}
func stopConditionMet(cond string, sc ScreenContext) bool {
	cond = strings.ToLower(strings.TrimSpace(cond))
	if cond == "" {
		return false
	}
	return strings.Contains(strings.ToLower(sc.RawText+" "+sc.Summary), cond)
}

func PlanDesktopLaunchOrBrowserTask(inst string) (ActionPlan, bool) {
	low := strings.ToLower(strings.TrimSpace(inst))
	if !(strings.Contains(low, "browser") || strings.Contains(low, "firefox") || strings.Contains(low, "chrome") || strings.Contains(low, "cari ") || strings.Contains(low, "search ")) {
		return ActionPlan{}, false
	}
	query := extractSearchQuery(inst)
	if query == "" && !(strings.Contains(low, "buka browser") || strings.Contains(low, "open browser")) {
		return ActionPlan{}, false
	}
	plan := ActionPlan{Instruction: inst, Mode: ModePlanOnly, Summary: "Browser automation khusus: buka browser, fokus address bar, ketik query/URL, lalu Enter."}
	plan.Actions = append(plan.Actions, PlannedAction{Type: "open_app", Value: "browser", Reason: "Membuka browser default untuk task web.", Risk: "low"})
	if query != "" {
		plan.Actions = append(plan.Actions, PlannedAction{Type: "key", Value: "ctrl+l", Reason: "Fokus address bar browser.", Risk: "low"})
		plan.Actions = append(plan.Actions, PlannedAction{Type: "type", Value: query, Reason: "Mengetik query/URL ke address bar.", RequiresConfirmation: true, Risk: "medium"})
		plan.Actions = append(plan.Actions, PlannedAction{Type: "key", Value: "Return", Reason: "Menjalankan pencarian/navigasi.", Risk: "low"})
	}
	return plan, true
}
func extractSearchQuery(inst string) string {
	low := strings.ToLower(inst)
	for _, sep := range []string{" cari ", " search ", " tentang ", " for "} {
		if idx := strings.Index(low, sep); idx >= 0 {
			return strings.TrimSpace(inst[idx+len(sep):])
		}
	}
	return ""
}
func extractAppName(inst string) string {
	low := strings.ToLower(strings.TrimSpace(inst))
	for _, prefix := range []string{"buka aplikasi ", "buka ", "open app ", "open "} {
		if strings.HasPrefix(low, prefix) {
			return strings.TrimSpace(inst[len(prefix):])
		}
	}
	return ""
}

func (DesktopExecutor) OpenApp(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("nama aplikasi kosong")
	}
	candidates := []string{name}
	if strings.EqualFold(name, "browser") {
		candidates = []string{"xdg-open", "firefox", "google-chrome", "chromium"}
	}
	if candidates[0] == "xdg-open" {
		return "xdg-open", exec.CommandContext(ctx, "xdg-open", "about:blank").Start()
	}
	if _, err := exec.LookPath("gtk-launch"); err == nil && !strings.Contains(name, "/") {
		if err := exec.CommandContext(ctx, "gtk-launch", name).Start(); err == nil {
			return "gtk-launch", nil
		}
	}
	for _, c := range candidates {
		if c == "xdg-open" {
			continue
		}
		if _, err := exec.LookPath(c); err == nil {
			return c, exec.CommandContext(ctx, c).Start()
		}
	}
	return "", fmt.Errorf("aplikasi %q tidak ditemukan", name)
}
