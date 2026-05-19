package voice

import "testing"

func TestPlanCommandMagicPointer(t *testing.T) {
	p := PlanCommand(CommandRequest{Transcript: "Smara, buka browser dan cari dokumentasi React", Autopilot: true, MaxSteps: 7})
	if p.Intent != "magic_pointer" {
		t.Fatalf("intent=%s", p.Intent)
	}
	joined := ""
	for _, a := range p.MagicPointerArgs {
		joined += a + " "
	}
	if !contains(joined, "--execute") || !contains(joined, "--max-steps 7") {
		t.Fatalf("args=%v", p.MagicPointerArgs)
	}
}

func TestDefaultSettings(t *testing.T) {
	s := NormalizeSettings(Settings{})
	if s.Language != "id-ID" || s.Provider != ProviderBrowser || s.Speed != 1 || s.Volume != 1 {
		t.Fatalf("bad defaults: %+v", s)
	}
}

func TestCleanupWhisperOutput(t *testing.T) {
	got := CleanupWhisperOutput("[00:00:00.000 --> 00:00:01.000] halo smara\nwhisper log")
	if got != "halo smara" {
		t.Fatalf("got %q", got)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && index(s, sub) >= 0) }
func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
