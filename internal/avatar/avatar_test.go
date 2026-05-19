package avatar

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Fatal("avatar should be enabled by default")
	}
	if cfg.State != StateIdle {
		t.Fatalf("state = %s", cfg.State)
	}
	if cfg.Expression == "" || cfg.SpeechBubble == "" {
		t.Fatal("expected expression and speech bubble")
	}
}

func TestApplyEventPriorityAndExpression(t *testing.T) {
	cfg := ApplyEvent(DefaultConfig(), Event{Listening: true, Speaking: true, Message: "Saya sedang berbicara."})
	if cfg.State != StateSpeaking {
		t.Fatalf("state = %s", cfg.State)
	}
	if cfg.Expression != "talking-smile" {
		t.Fatalf("expression = %s", cfg.Expression)
	}
	if cfg.SpeechBubble != "Saya sedang berbicara." {
		t.Fatalf("bubble = %q", cfg.SpeechBubble)
	}

	cfg = ApplyEvent(cfg, Event{Emergency: true, Error: true})
	if cfg.State != StateEmergencyStop {
		t.Fatalf("emergency should win, got %s", cfg.State)
	}
	if cfg.Expression != "alert" {
		t.Fatalf("expression = %s", cfg.Expression)
	}
}

func TestNormalizeState(t *testing.T) {
	if NormalizeState(State("weird")) != StateIdle {
		t.Fatal("invalid state should normalize to idle")
	}
	if !ValidState(StateActing) {
		t.Fatal("acting should be valid")
	}
}
