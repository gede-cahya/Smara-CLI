package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Provider string

const (
	ProviderAuto    Provider = "auto"
	ProviderBrowser Provider = "browser"
	ProviderWhisper Provider = "whisper"
	ProviderPiper   Provider = "piper"
	ProviderMock    Provider = "mock"
)

type Settings struct {
	Provider       Provider `json:"provider"`
	Language       string   `json:"language"`
	VoiceCharacter string   `json:"voice_character"`
	Speed          float64  `json:"speed"`
	Volume         float64  `json:"volume"`
	Streaming      bool     `json:"streaming"`
}

type CommandRequest struct {
	Transcript string   `json:"transcript"`
	Language   string   `json:"language"`
	Autopilot  bool     `json:"autopilot"`
	MaxSteps   int      `json:"max_steps"`
	Source     string   `json:"source"`
	Guardrails []string `json:"guardrails,omitempty"`
}

type CommandPlan struct {
	Transcript       string   `json:"transcript"`
	Intent           string   `json:"intent"`
	MagicPointerArgs []string `json:"magic_pointer_args,omitempty"`
	NeedsGuardrail   bool     `json:"needs_guardrail"`
	Warnings         []string `json:"warnings,omitempty"`
}

type SynthesisRequest struct {
	Text     string   `json:"text"`
	Settings Settings `json:"settings"`
	Output   string   `json:"output,omitempty"`
}

type SynthesisResult struct {
	AudioPath string `json:"audio_path,omitempty"`
	Provider  string `json:"provider"`
	Text      string `json:"text"`
	Error     string `json:"error,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{Provider: ProviderBrowser, Language: "id-ID", VoiceCharacter: "Smara", Speed: 1, Volume: 1, Streaming: true}
}

func NormalizeSettings(s Settings) Settings {
	if s.Provider == "" {
		s.Provider = ProviderBrowser
	}
	if s.Language == "" {
		s.Language = "id-ID"
	}
	if s.VoiceCharacter == "" {
		s.VoiceCharacter = "Smara"
	}
	if s.Speed <= 0 {
		s.Speed = 1
	}
	if s.Volume <= 0 {
		s.Volume = 1
	}
	if s.Volume > 1 {
		s.Volume = 1
	}
	return s
}

func PlanCommand(req CommandRequest) CommandPlan {
	text := strings.TrimSpace(req.Transcript)
	p := CommandPlan{Transcript: text, Intent: "chat", NeedsGuardrail: req.Autopilot}
	if text == "" {
		p.Warnings = append(p.Warnings, "transkrip kosong")
		return p
	}
	low := strings.ToLower(text)
	if strings.Contains(low, "buka") || strings.Contains(low, "klik") || strings.Contains(low, "ketik") || strings.Contains(low, "cari") || strings.Contains(low, "browser") || strings.Contains(low, "desktop") {
		p.Intent = "magic_pointer"
		maxSteps := req.MaxSteps
		if maxSteps <= 0 {
			maxSteps = 10
		}
		p.MagicPointerArgs = []string{"magic-pointer", "--ask", text, "--autopilot", "--max-steps", strconv.Itoa(maxSteps)}
		if req.Autopilot {
			p.MagicPointerArgs = append(p.MagicPointerArgs, "--execute")
			p.Warnings = append(p.Warnings, "autopilot desktop wajib tetap memakai emergency stop dan audit log")
		}
	}
	return p
}

func Transcribe(ctx context.Context, audio string, provider Provider, language string) (string, string, error) {
	if strings.TrimSpace(audio) == "" {
		return "", "", errors.New("audio kosong")
	}
	if provider == ProviderMock {
		b, err := os.ReadFile(audio)
		if err != nil {
			return "", "mock", err
		}
		return strings.TrimSpace(string(b)), "mock", nil
	}
	if language == "" {
		language = "Indonesian"
	}
	if provider == ProviderAuto || provider == ProviderWhisper {
		if _, err := exec.LookPath("whisper"); err == nil {
			b, e := exec.CommandContext(ctx, "whisper", audio, "--model", "base", "--language", language, "--fp16", "False", "--output_format", "txt", "--output_dir", filepath.Dir(audio)).CombinedOutput()
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
				return CleanupWhisperOutput(string(b)), bin, nil
			}
		}
	}
	return "", "", errors.New("STT tidak tersedia: gunakan browser Web Speech API, whisper/openai-whisper, whisper.cpp, atau provider mock")
}

func CleanupWhisperOutput(s string) string {
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

func Synthesize(ctx context.Context, req SynthesisRequest) SynthesisResult {
	s := NormalizeSettings(req.Settings)
	text := strings.TrimSpace(req.Text)
	res := SynthesisResult{Provider: string(s.Provider), Text: text}
	if text == "" {
		res.Error = "teks kosong"
		return res
	}
	if s.Provider == ProviderBrowser {
		res.Provider = "browser"
		return res
	}
	out := req.Output
	if out == "" {
		out = filepath.Join(os.TempDir(), "smara-voice-"+time.Now().Format("20060102-150405")+".wav")
	}
	if s.Provider == ProviderAuto || s.Provider == ProviderPiper {
		if _, err := exec.LookPath("piper"); err == nil {
			cmd := exec.CommandContext(ctx, "piper", "--output_file", out)
			cmd.Stdin = strings.NewReader(text)
			if b, err := cmd.CombinedOutput(); err != nil {
				res.Error = fmt.Sprintf("piper gagal: %s", strings.TrimSpace(string(b)))
				return res
			}
			res.Provider = "piper"
			res.AudioPath = out
			return res
		}
	}
	res.Error = "TTS lokal tidak tersedia: gunakan browser speechSynthesis atau install piper"
	return res
}
