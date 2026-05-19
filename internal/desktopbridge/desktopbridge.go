package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/magicpointer"
)

type Options struct {
	Addr         string
	AuditLog     string
	Mode         string
	AllowCommand []string
	Token        string
}
type Service struct {
	Opt     Options
	stopped atomic.Bool
}

type AuditEvent struct {
	Timestamp time.Time   `json:"timestamp"`
	Action    string      `json:"action"`
	Target    string      `json:"target,omitempty"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type Window struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

type response struct {
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func New(opt Options) *Service {
	if opt.Addr == "" {
		opt.Addr = "127.0.0.1:8765"
	}
	if opt.Mode == "" {
		opt.Mode = "supervised"
	}
	if opt.AuditLog == "" {
		if h, _ := os.UserHomeDir(); h != "" {
			opt.AuditLog = filepath.Join(h, ".smara", "desktop-agent", "audit.jsonl")
		}
	}
	return &Service{Opt: opt}
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.wrap("health", func(r *http.Request) (interface{}, error) {
		return map[string]interface{}{"status": "ok", "mode": s.Opt.Mode, "stopped": s.stopped.Load()}, nil
	}))
	mux.HandleFunc("/stop", s.wrap("stop", func(r *http.Request) (interface{}, error) { s.stopped.Store(true); return "emergency stop active", nil }))
	mux.HandleFunc("/resume", s.wrap("resume", func(r *http.Request) (interface{}, error) { s.stopped.Store(false); return "resumed", nil }))
	mux.HandleFunc("/screenshot", s.guard("screenshot", func(r *http.Request) (interface{}, error) {
		p, err := magicpointer.CaptureScreenshot(r.Context(), "")
		return map[string]string{"path": p}, err
	}))
	mux.HandleFunc("/window/active", s.wrap("active_window", func(r *http.Request) (interface{}, error) { return ActiveWindow(r.Context()) }))
	mux.HandleFunc("/windows", s.wrap("list_windows", func(r *http.Request) (interface{}, error) { return ListWindows(r.Context()) }))
	mux.HandleFunc("/window/focus", s.guard("focus_window", func(r *http.Request) (interface{}, error) {
		var in struct {
			ID string `json:"id"`
		}
		decode(r, &in)
		return "focused", FocusWindow(r.Context(), in.ID)
	}))
	mux.HandleFunc("/app/open", s.guard("open_app", func(r *http.Request) (interface{}, error) {
		var in struct {
			Name string `json:"name"`
		}
		decode(r, &in)
		return "opened", OpenApp(r.Context(), in.Name)
	}))
	mux.HandleFunc("/clipboard/read", s.wrap("clipboard_read", func(r *http.Request) (interface{}, error) { return ReadClipboard(r.Context()) }))
	mux.HandleFunc("/clipboard/write", s.guard("clipboard_write", func(r *http.Request) (interface{}, error) {
		var in struct {
			Text string `json:"text"`
		}
		decode(r, &in)
		return "written", WriteClipboard(r.Context(), in.Text)
	}))
	mux.HandleFunc("/mouse/move", s.guard("mouse_move", func(r *http.Request) (interface{}, error) {
		var in struct{ X, Y int }
		decode(r, &in)
		return "moved", MoveMouse(r.Context(), in.X, in.Y)
	}))
	mux.HandleFunc("/click", s.guard("click", func(r *http.Request) (interface{}, error) {
		var in struct {
			X, Y   int
			Button string
		}
		decode(r, &in)
		return "clicked", Click(r.Context(), in.X, in.Y, in.Button)
	}))
	mux.HandleFunc("/double-click", s.guard("double_click", func(r *http.Request) (interface{}, error) {
		var in struct{ X, Y int }
		decode(r, &in)
		return "double-clicked", DoubleClick(r.Context(), in.X, in.Y)
	}))
	mux.HandleFunc("/right-click", s.guard("right_click", func(r *http.Request) (interface{}, error) {
		var in struct{ X, Y int }
		decode(r, &in)
		return "right-clicked", Click(r.Context(), in.X, in.Y, "3")
	}))
	mux.HandleFunc("/scroll", s.guard("scroll", func(r *http.Request) (interface{}, error) {
		var in struct{ Direction string }
		decode(r, &in)
		tool, err := magicpointer.DesktopExecutor{}.Scroll(r.Context(), in.Direction)
		return map[string]string{"status": "scrolled", "tool": tool}, err
	}))
	mux.HandleFunc("/type", s.guard("type", func(r *http.Request) (interface{}, error) {
		var in struct {
			Text string `json:"text"`
		}
		decode(r, &in)
		tool, err := magicpointer.DesktopExecutor{}.TypeText(r.Context(), in.Text)
		return map[string]string{"status": "typed", "tool": tool}, err
	}))
	mux.HandleFunc("/hotkey", s.guard("hotkey", func(r *http.Request) (interface{}, error) {
		var in struct {
			Key string `json:"key"`
		}
		decode(r, &in)
		tool, err := magicpointer.DesktopExecutor{}.Key(r.Context(), in.Key)
		return map[string]string{"status": "pressed", "tool": tool}, err
	}))
	mux.HandleFunc("/command/run", s.guard("command_run", func(r *http.Request) (interface{}, error) {
		var in struct {
			Name string   `json:"name"`
			Args []string `json:"args"`
		}
		decode(r, &in)
		return RunAllowedCommand(r.Context(), s.Opt.AllowCommand, in.Name, in.Args)
	}))
	mux.HandleFunc("/task/run", s.guard("task_run", func(r *http.Request) (interface{}, error) {
		var in struct {
			Instruction   string `json:"instruction"`
			MaxSteps      int    `json:"max_steps"`
			StopCondition string `json:"stop_condition"`
			AssumeYes     bool   `json:"assume_yes"`
		}
		decode(r, &in)
		return magicpointer.RunAutopilot(r.Context(), magicpointer.AutopilotOptions{Options: magicpointer.Options{Instruction: in.Instruction, Execute: true, AssumeYes: in.AssumeYes, AuditLogPath: s.Opt.AuditLog, RedactSensitive: true}, MaxSteps: in.MaxSteps, StopCondition: in.StopCondition})
	}))
	mux.HandleFunc("/screenshot.png", func(w http.ResponseWriter, r *http.Request) {
		if s.stopped.Load() {
			err := errors.New("desktop-agent emergency stop active")
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			_ = s.audit("screenshot_png", nil, err)
			return
		}
		p, err := magicpointer.CaptureScreenshot(r.Context(), "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			_ = s.audit("screenshot_png", nil, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, p)
		_ = s.audit("screenshot_png", map[string]string{"path": p}, nil)
	})
	return s.withAuth(mux)
}

func (s *Service) wrap(action string, fn func(*http.Request) (interface{}, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		res, err := fn(r)
		out := response{OK: err == nil, Result: res}
		if err != nil {
			out.Error = err.Error()
			w.WriteHeader(500)
		}
		_ = s.audit(action, res, err)
		_ = json.NewEncoder(w).Encode(out)
	}
}
func (s *Service) guard(action string, fn func(*http.Request) (interface{}, error)) http.HandlerFunc {
	return s.wrap(action, func(r *http.Request) (interface{}, error) {
		if s.stopped.Load() && action != "resume" && action != "health" {
			return nil, errors.New("desktop-agent emergency stop active")
		}
		return fn(r)
	})
}

func (s *Service) withAuth(next http.Handler) http.Handler {
	if strings.TrimSpace(s.Opt.Token) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != s.Opt.Token {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(response{OK: false, Error: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) audit(action string, result interface{}, err error) error {
	if strings.TrimSpace(s.Opt.AuditLog) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Opt.AuditLog), 0o755); err != nil {
		return err
	}
	event := AuditEvent{Timestamp: time.Now(), Action: action, Result: result}
	if err != nil {
		event.Error = err.Error()
	}
	b, e := json.Marshal(event)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(s.Opt.AuditLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write(append(b, '\n'))
	return e
}

func (s *Service) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{Addr: s.Opt.Addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func decode(r *http.Request, v interface{}) { _ = json.NewDecoder(r.Body).Decode(v) }

func ActiveWindow(ctx context.Context) (Window, error) {
	if _, e := exec.LookPath("xdotool"); e == nil {
		idb, e := exec.CommandContext(ctx, "xdotool", "getactivewindow").Output()
		if e != nil {
			return Window{}, e
		}
		id := strings.TrimSpace(string(idb))
		tb, _ := exec.CommandContext(ctx, "xdotool", "getwindowname", id).Output()
		return Window{ID: id, Title: strings.TrimSpace(string(tb))}, nil
	}
	return Window{}, errors.New("active window backend tidak tersedia: install xdotool")
}

func ListWindows(ctx context.Context) ([]Window, error) {
	if _, e := exec.LookPath("wmctrl"); e == nil {
		b, e := exec.CommandContext(ctx, "wmctrl", "-l").Output()
		if e != nil {
			return nil, e
		}
		var out []Window
		for _, ln := range strings.Split(string(b), "\n") {
			f := strings.Fields(ln)
			if len(f) >= 4 {
				out = append(out, Window{ID: f[0], Title: strings.Join(f[3:], " ")})
			}
		}
		return out, nil
	}
	if _, e := exec.LookPath("xdotool"); e == nil {
		b, e := exec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--name", ".").Output()
		if e != nil {
			return nil, e
		}
		var out []Window
		for _, id := range strings.Fields(string(b)) {
			tb, _ := exec.CommandContext(ctx, "xdotool", "getwindowname", id).Output()
			out = append(out, Window{ID: id, Title: strings.TrimSpace(string(tb))})
		}
		return out, nil
	}
	return nil, errors.New("list window backend tidak tersedia: install wmctrl atau xdotool")
}
func FocusWindow(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("id window kosong")
	}
	if _, e := exec.LookPath("xdotool"); e == nil {
		return exec.CommandContext(ctx, "xdotool", "windowactivate", id).Run()
	}
	return errors.New("focus backend tidak tersedia: install xdotool")
}
func OpenApp(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("nama aplikasi kosong")
	}
	return exec.CommandContext(ctx, "gtk-launch", name).Start()
}
func ReadClipboard(ctx context.Context) (string, error) {
	for _, c := range [][]string{{"wl-paste"}, {"xclip", "-selection", "clipboard", "-o"}, {"xsel", "-b", "-o"}} {
		if _, e := exec.LookPath(c[0]); e == nil {
			b, e := exec.CommandContext(ctx, c[0], c[1:]...).Output()
			return string(b), e
		}
	}
	return "", errors.New("clipboard read backend tidak tersedia")
}
func WriteClipboard(ctx context.Context, text string) error {
	for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "-b", "-i"}} {
		if _, e := exec.LookPath(c[0]); e == nil {
			cmd := exec.CommandContext(ctx, c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	return errors.New("clipboard write backend tidak tersedia")
}
func MoveMouse(ctx context.Context, x, y int) error {
	if _, e := exec.LookPath("xdotool"); e == nil {
		return exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y)).Run()
	}
	if _, e := exec.LookPath("ydotool"); e == nil {
		return exec.CommandContext(ctx, "ydotool", "mousemove", "-a", strconv.Itoa(x), strconv.Itoa(y)).Run()
	}
	return errors.New("mouse backend tidak tersedia")
}
func Click(ctx context.Context, x, y int, button string) error {
	if button == "" {
		button = "1"
	}
	if e := MoveMouse(ctx, x, y); e != nil {
		return e
	}
	if _, e := exec.LookPath("xdotool"); e == nil {
		return exec.CommandContext(ctx, "xdotool", "click", button).Run()
	}
	if _, e := exec.LookPath("ydotool"); e == nil {
		return exec.CommandContext(ctx, "ydotool", "click", button).Run()
	}
	return errors.New("click backend tidak tersedia")
}
func DoubleClick(ctx context.Context, x, y int) error {
	if e := Click(ctx, x, y, "1"); e != nil {
		return e
	}
	return Click(ctx, x, y, "1")
}
func RunAllowedCommand(ctx context.Context, allow []string, name string, args []string) (string, error) {
	name = strings.TrimSpace(name)
	ok := false
	for _, a := range allow {
		if a == name {
			ok = true
		}
	}
	if !ok {
		return "", fmt.Errorf("command %q tidak ada di allowlist", name)
	}
	b, e := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(b), e
}
