package browser

import (
	"strings"
	"testing"
)

func TestIsBrowserPrompt(t *testing.T) {
	if !IsBrowserPrompt("Buka browser ke http://localhost:3000 dan ambil screenshot") {
		t.Fatal("expected browser prompt")
	}
	if !IsBrowserPrompt("Buka http://127.0.0.1:8080 klik Chat ambil screenshot") {
		t.Fatal("expected click browser prompt")
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

func TestPlanClickText(t *testing.T) {
	task, err := Plan("Buka http://127.0.0.1:8080/ klik Chat ambil screenshot")
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Steps) < 3 {
		t.Fatalf("steps=%+v", task.Steps)
	}
	if task.Steps[1].Action != "click" || task.Steps[1].Target != "Chat" {
		t.Fatalf("click step=%+v", task.Steps[1])
	}
	last := task.Steps[len(task.Steps)-1]
	if last.Action != "screenshot" {
		t.Fatalf("last=%+v", last)
	}
}

func TestPlanWaitText(t *testing.T) {
	task, err := Plan("Open http://localhost:8080 click Chat wait Halo screenshot")
	if err != nil {
		t.Fatal(err)
	}
	var hasClick, hasWait bool
	for _, step := range task.Steps {
		if step.Action == "click" && step.Target == "Chat" {
			hasClick = true
		}
		if step.Action == "wait" && step.Target == "Halo" {
			hasWait = true
		}
	}
	if !hasClick || !hasWait {
		t.Fatalf("steps=%+v", task.Steps)
	}
}
