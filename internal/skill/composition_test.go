package skill

import (
	"errors"
	"testing"
)

func TestDependencyWorkflowParsingAndPlan(t *testing.T) {
	sk := &Skill{Name: "deploy", Dependencies: []string{"build"}, DependencyWorkflow: &DependencyWorkflow{
		Requires:  []string{"lint"},
		Suggests:  []string{"monitor"},
		Precheck:  []string{"health"},
		Postcheck: []string{"verify"},
	}}
	wf := sk.WorkflowDependencies()
	if len(wf.Requires) != 2 || wf.Requires[0] != "lint" || wf.Requires[1] != "build" {
		t.Fatalf("requires not merged as expected: %#v", wf.Requires)
	}
	plan := sk.CompositionPlan()
	wantKinds := []string{"requires", "requires", "precheck", "main", "postcheck"}
	if len(plan.Steps) != len(wantKinds) {
		t.Fatalf("unexpected step count: %d", len(plan.Steps))
	}
	for i, kind := range wantKinds {
		if plan.Steps[i].Kind != kind {
			t.Fatalf("step %d kind=%s want=%s", i, plan.Steps[i].Kind, kind)
		}
	}
	if len(plan.Suggests) != 1 || plan.Suggests[0] != "monitor" {
		t.Fatalf("suggests missing from plan: %#v", plan.Suggests)
	}
}

func TestRunComposedPrecheckSuccessAndPostcheckRuns(t *testing.T) {
	order := []string{}
	skills := map[string]*Skill{
		"health": {Name: "health", Steps: []Step{{Tool: "health"}}},
		"verify": {Name: "verify", Steps: []Step{{Tool: "verify"}}},
	}
	main := &Skill{Name: "deploy", Steps: []Step{{Tool: "deploy"}}, DependencyWorkflow: &DependencyWorkflow{Precheck: []string{"health"}, Postcheck: []string{"verify"}}}
	res, err := main.RunComposed(func(name string) (*Skill, error) { return skills[name], nil }, func(tool string, args map[string]interface{}) (string, error) {
		order = append(order, tool)
		return "ok", nil
	})
	if err != nil || !res.Success {
		t.Fatalf("workflow failed: res=%#v err=%v", res, err)
	}
	want := []string{"health", "deploy", "verify"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order=%v want=%v", order, want)
		}
	}
}

func TestRunComposedPrecheckFailureBlocksMain(t *testing.T) {
	order := []string{}
	skills := map[string]*Skill{"health": {Name: "health", Steps: []Step{{Tool: "health"}}}}
	main := &Skill{Name: "deploy", Steps: []Step{{Tool: "deploy"}}, DependencyWorkflow: &DependencyWorkflow{Precheck: []string{"health"}}}
	res, err := main.RunComposed(func(name string) (*Skill, error) { return skills[name], nil }, func(tool string, args map[string]interface{}) (string, error) {
		order = append(order, tool)
		if tool == "health" {
			return "", errors.New("down")
		}
		return "ok", nil
	})
	if err != nil || res.Success {
		t.Fatalf("expected blocked workflow: res=%#v err=%v", res, err)
	}
	if len(order) != 1 || order[0] != "health" {
		t.Fatalf("main should not run, order=%v", order)
	}
}

func TestRunComposedSuggestionDoesNotRunOrBlock(t *testing.T) {
	main := &Skill{Name: "deploy", Steps: []Step{{Tool: "deploy"}}, DependencyWorkflow: &DependencyWorkflow{Suggests: []string{"optional"}}}
	res, err := main.RunComposed(func(name string) (*Skill, error) { return nil, errors.New("missing") }, func(tool string, args map[string]interface{}) (string, error) {
		return "ok", nil
	})
	if err != nil || !res.Success {
		t.Fatalf("suggestion should not block: res=%#v err=%v", res, err)
	}
	if len(res.Results) != 1 || res.Results[0].SkillName != "deploy" {
		t.Fatalf("suggestion should not execute, results=%#v", res.Results)
	}
}
