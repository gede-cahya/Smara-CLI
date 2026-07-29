package discord

import "testing"

func TestPRDWizardTransitions(t *testing.T) {
	store := newPRDWizardStore()
	sess := store.start("guild", "channel", "user-1", "cahya", "aplikasi booking lapangan")
	steps := []struct {
		value string
		want  prdStep
	}{
		{"webapp", prdStepTargetUser},
		{"consumer", prdStepPlatform},
		{"web", prdStepScope},
		{"mvp", prdStepWorkflowPlan},
		{"agile", prdStepDiagramFlow},
		{"flowchart_seq", prdStepDetail},
		{"full", prdStepDone},
	}
	for _, step := range steps {
		updated, allowed, err := store.apply(sess.ID, "user-1", step.value)
		if err != nil || !allowed {
			t.Fatalf("apply failed: allowed=%v err=%v", allowed, err)
		}
		if updated.Step != step.want {
			t.Fatalf("after %s step=%d want=%d", step.value, updated.Step, step.want)
		}
	}
	if sess.Answers.ProductType != "Web App" || sess.Answers.DetailLevel != "Lengkap" || sess.Answers.WorkflowPlan != "Agile Sprints" || sess.Answers.DiagramFlow != "Flowchart & Sequence" {
		t.Fatalf("answers not mapped: %+v", sess.Answers)
	}
}

func TestPRDWizardRejectsOtherUser(t *testing.T) {
	store := newPRDWizardStore()
	sess := store.start("guild", "channel", "owner", "owner", "ide")
	_, allowed, err := store.apply(sess.ID, "other", "webapp")
	if err == nil || allowed {
		t.Fatalf("expected rejection for other user")
	}
}

func TestParsePRDButtonID(t *testing.T) {
	sessionID, value, ok := parsePRDButtonID("smara_prd:abc123:mvp")
	if !ok || sessionID != "abc123" || value != "mvp" {
		t.Fatalf("unexpected parse: %s %s %v", sessionID, value, ok)
	}
}
