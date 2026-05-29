package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

type Provider string

const (
	ProviderAuto       Provider = "auto"
	ProviderBrowser    Provider = "browser"
	ProviderWhisper    Provider = "whisper"
	ProviderPiper      Provider = "piper"
	ProviderElevenLabs Provider = "elevenlabs"
	ProviderMock       Provider = "mock"
)

type Settings struct {
	Provider       Provider `json:"provider"`
	Language       string   `json:"language"`
	VoiceCharacter string   `json:"voice_character"`
	ModelID        string   `json:"model_id"`
	Speed          float64  `json:"speed"`
	Volume         float64  `json:"volume"`
	Streaming      bool     `json:"streaming"`
	APIKey         string   `json:"api_key,omitempty"`
	BaseURL        string   `json:"base_url,omitempty"`
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

type AudioResult struct {
	Data        []byte
	ContentType string
	Provider    string
}

func DefaultSettings() Settings {
	provider := ProviderBrowser
	if cfgProvider := strings.TrimSpace(fmt.Sprint(config.GetValue("voice_provider"))); cfgProvider != "" && cfgProvider != "<nil>" {
		provider = Provider(cfgProvider)
	}
	return NormalizeSettings(Settings{
		Provider:       provider,
		Language:       configString("voice_language", "id-ID"),
		VoiceCharacter: configString("voice_character", envOr("ELEVENLABS_VOICE_ID", "")),
		ModelID:        configString("voice_model_id", envOr("ELEVENLABS_MODEL_ID", "eleven_multilingual_v2")),
		Speed:          configFloat("voice_speed", 1),
		Volume:         configFloat("voice_volume", 1),
		Streaming:      configBool("voice_streaming", true),
		BaseURL:        configString("voice_base_url", envOr("ELEVENLABS_BASE_URL", "https://api.elevenlabs.io")),
	})
}

func NormalizeSettings(s Settings) Settings {
	defaults := Settings{
		Provider:       Provider(configString("voice_provider", "")),
		Language:       configString("voice_language", "id-ID"),
		VoiceCharacter: configString("voice_character", envOr("ELEVENLABS_VOICE_ID", "")),
		ModelID:        configString("voice_model_id", envOr("ELEVENLABS_MODEL_ID", "eleven_multilingual_v2")),
		Speed:          configFloat("voice_speed", 1),
		Volume:         configFloat("voice_volume", 1),
		Streaming:      configBool("voice_streaming", true),
		BaseURL:        configString("voice_base_url", envOr("ELEVENLABS_BASE_URL", "https://api.elevenlabs.io")),
	}
	if s.Provider == "" {
		if defaults.Provider != "" {
			s.Provider = defaults.Provider
		} else {
			s.Provider = ProviderBrowser
		}
	}
	if s.Language == "" {
		s.Language = defaults.Language
	}
	if s.VoiceCharacter == "" {
		s.VoiceCharacter = defaults.VoiceCharacter
	}
	if s.ModelID == "" {
		s.ModelID = defaults.ModelID
	}
	if s.Speed <= 0 {
		s.Speed = defaults.Speed
	}
	if s.Volume <= 0 {
		s.Volume = defaults.Volume
	}
	if s.Volume > 1 {
		s.Volume = 1
	}
	if strings.TrimSpace(s.BaseURL) == "" {
		s.BaseURL = defaults.BaseURL
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
		ext := ".wav"
		if s.Provider == ProviderElevenLabs {
			ext = ".mp3"
		}
		out = filepath.Join(os.TempDir(), "smara-voice-"+time.Now().Format("20060102-150405")+ext)
	}
	if s.Provider == ProviderElevenLabs {
		audio, err := SynthesizeAudio(ctx, req)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		if err := os.WriteFile(out, audio.Data, 0600); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Provider = audio.Provider
		res.AudioPath = out
		return res
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
	res.Error = "TTS lokal tidak tersedia: gunakan browser speechSynthesis, ElevenLabs, atau install piper"
	return res
}

func SynthesizeAudio(ctx context.Context, req SynthesisRequest) (AudioResult, error) {
	s := NormalizeSettings(req.Settings)
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return AudioResult{}, errors.New("teks kosong")
	}
	if s.Provider != ProviderElevenLabs {
		return AudioResult{}, errors.New("provider audio backend harus elevenlabs")
	}
	apiKey := strings.TrimSpace(s.APIKey)
	if apiKey == "" {
		apiKey = configString("voice_api_key", envOr("ELEVENLABS_API_KEY", ""))
	}
	if apiKey == "" {
		return AudioResult{}, errors.New("voice_api_key belum diset di config atau ELEVENLABS_API_KEY belum diset di environment backend")
	}
	voiceID := strings.TrimSpace(s.VoiceCharacter)
	if voiceID == "" {
		voiceID = envOr("ELEVENLABS_VOICE_ID", "")
	}
	if voiceID == "" {
		return AudioResult{}, errors.New("voice_character / ELEVENLABS_VOICE_ID belum diset. Pilih voice dari akun ElevenLabs milik sendiri; free plan tidak bisa memakai library voice via API")
	}
	model := strings.TrimSpace(s.ModelID)
	if model == "" {
		model = configString("voice_model_id", envOr("ELEVENLABS_MODEL_ID", "eleven_multilingual_v2"))
	}
	baseURL := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.elevenlabs.io"
	}
	payload := map[string]interface{}{"text": text, "model_id": model, "voice_settings": map[string]float64{"stability": 0.5, "similarity_boost": 0.75}}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/text-to-speech/"+voiceID, bytes.NewReader(body))
	if err != nil {
		return AudioResult{}, err
	}
	httpReq.Header.Set("xi-api-key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return AudioResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return AudioResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if resp.StatusCode == http.StatusPaymentRequired && (strings.Contains(strings.ToLower(msg), "paid_plan_required") || strings.Contains(strings.ToLower(msg), "library voices")) {
			return AudioResult{}, fmt.Errorf("ElevenLabs voice ini butuh paid plan/library voice. Ganti voice_character ke voice ID dari akun sendiri atau ubah provider ke browser. Detail: %s", msg)
		}
		return AudioResult{}, fmt.Errorf("ElevenLabs gagal (%d): %s", resp.StatusCode, msg)
	}
	return AudioResult{Data: data, ContentType: "audio/mpeg", Provider: "elevenlabs"}, nil
}

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func configString(key, fallback string) string {
	v := strings.TrimSpace(fmt.Sprint(config.GetValue(key)))
	if v == "" || v == "<nil>" {
		return fallback
	}
	return v
}

func configFloat(key string, fallback float64) float64 {
	s := configString(key, "")
	if s == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return f
}

func configBool(key string, fallback bool) bool {
	s := configString(key, "")
	if s == "" {
		return fallback
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fallback
	}
	return b
}
