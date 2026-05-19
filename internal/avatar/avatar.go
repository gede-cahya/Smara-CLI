package avatar

import "strings"

// State is the high-level embodied assistant state mirrored by Smara Web.
type State string

const (
	StateIdle            State = "idle"
	StateListening       State = "listening"
	StateThinking        State = "thinking"
	StateSpeaking        State = "speaking"
	StateActing          State = "acting"
	StateWaitingApproval State = "waiting_approval"
	StateSuccess         State = "success"
	StateError           State = "error"
	StateEmergencyStop   State = "emergency_stop"
)

// Config describes the web avatar MVP settings.
type Config struct {
	Enabled       bool    `json:"enabled"`
	Model         string  `json:"model"`
	Style         string  `json:"style"`
	State         State   `json:"state"`
	Expression    string  `json:"expression"`
	SpeechBubble  string  `json:"speech_bubble"`
	LipSync       bool    `json:"lip_sync"`
	Gesture       string  `json:"gesture"`
	LightMode     bool    `json:"light_mode"`
	VoiceReactive bool    `json:"voice_reactive"`
	Intensity     float64 `json:"intensity"`
}

// Event updates the avatar state from voice, agent, or Magic Pointer events.
type Event struct {
	State      State   `json:"state"`
	Message    string  `json:"message"`
	Speaking   bool    `json:"speaking"`
	Acting     bool    `json:"acting"`
	Listening  bool    `json:"listening"`
	Error      bool    `json:"error"`
	Success    bool    `json:"success"`
	Emergency  bool    `json:"emergency"`
	Gesture    string  `json:"gesture"`
	AudioLevel float64 `json:"audio_level"`
}

func DefaultConfig() Config {
	return Config{Enabled: true, Model: "smara-cyber-neko-placeholder", Style: "anime-3d-cyber-companion", State: StateIdle, Expression: "soft-smile", SpeechBubble: "Halo, saya Smara. Siap membantu.", LipSync: true, Gesture: "idle-float", VoiceReactive: true, Intensity: 0.35}
}

func ValidState(s State) bool {
	switch s {
	case StateIdle, StateListening, StateThinking, StateSpeaking, StateActing, StateWaitingApproval, StateSuccess, StateError, StateEmergencyStop:
		return true
	default:
		return false
	}
}

func NormalizeState(s State) State {
	if ValidState(s) {
		return s
	}
	return StateIdle
}

func ApplyEvent(cfg Config, ev Event) Config {
	if ev.Emergency {
		cfg.State = StateEmergencyStop
	} else if ev.Error {
		cfg.State = StateError
	} else if ev.Success {
		cfg.State = StateSuccess
	} else if ev.Speaking {
		cfg.State = StateSpeaking
	} else if ev.Acting {
		cfg.State = StateActing
	} else if ev.Listening {
		cfg.State = StateListening
	} else if ValidState(ev.State) {
		cfg.State = ev.State
	}
	if strings.TrimSpace(ev.Message) != "" {
		cfg.SpeechBubble = strings.TrimSpace(ev.Message)
	}
	if strings.TrimSpace(ev.Gesture) != "" {
		cfg.Gesture = strings.TrimSpace(ev.Gesture)
	}
	if ev.AudioLevel > 0 {
		cfg.Intensity = ev.AudioLevel
	}
	cfg.Expression = ExpressionForState(cfg.State)
	return cfg
}

func ExpressionForState(s State) string {
	switch NormalizeState(s) {
	case StateListening:
		return "attentive"
	case StateThinking:
		return "focused"
	case StateSpeaking:
		return "talking-smile"
	case StateActing:
		return "determined"
	case StateWaitingApproval:
		return "curious"
	case StateSuccess:
		return "happy"
	case StateError:
		return "concerned"
	case StateEmergencyStop:
		return "alert"
	default:
		return "soft-smile"
	}
}
