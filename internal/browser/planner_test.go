package browser

import (
	"strings"
	"testing"
)

func TestIsBrowserPrompt(t *testing.T) {
	if !IsBrowserPrompt("Buka browser ke http://localhost:3000 dan ambil screenshot") {
		t.Fatal("expected browser prompt")
	}
	if IsBrowserPrompt("tolong refactor file go") {
		t.Fatal("unexpected browser prompt")
	}
}

func TestPlanLogin(t *testing.T) {
	p := "Gunakan browser subagent untuk membuka http://localhost:3000. login username 'admin' password 'password123' ambil screenshot dashboard"
	task, err := Plan(p)
	if err != nil {
		t.Fatal(err)
	}
	if task.URL != "http://localhost:3000" {
		t.Fatalf("url=%s", task.URL)
	}
	if len(task.Steps) < 6 {
		t.Fatalf("steps=%d", len(task.Steps))
	}
	if task.Steps[1].Value != "admin" {
		t.Fatalf("username=%s", task.Steps[1].Value)
	}
	if task.Steps[2].Value != "password123" {
		t.Fatalf("password=%s", task.Steps[2].Value)
	}
}

func TestPlanMobileNavbar(t *testing.T) {
	task, err := Plan("Buka http://localhost:5173 di browser periksa navbar mobile ambil screenshot")
	if err != nil {
		t.Fatal(err)
	}
	if task.Viewport.Name != "mobile" {
		t.Fatalf("viewport=%s", task.Viewport.Name)
	}
	last := task.Steps[len(task.Steps)-1]
	if !strings.Contains(last.Name, "navbar") {
		t.Fatalf("last=%+v", last)
	}
}
