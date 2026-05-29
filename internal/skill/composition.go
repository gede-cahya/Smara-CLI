package skill

import "fmt"

// DependencyWorkflow defines typed skill dependencies for workflow composition.
type DependencyWorkflow struct {
	Requires  []string `json:"requires,omitempty"`
	Suggests  []string `json:"suggests,omitempty"`
	Precheck  []string `json:"precheck,omitempty"`
	Postcheck []string `json:"postcheck,omitempty"`
}

// CompositionStep describes one skill-level node in a composed run plan.
type CompositionStep struct {
	Kind      string `json:"kind"`
	SkillName string `json:"skill_name"`
	Blocking  bool   `json:"blocking"`
}

// CompositionPlan is the execution plan for a skill plus typed dependencies.
type CompositionPlan struct {
	MainSkill string            `json:"main_skill"`
	Steps     []CompositionStep `json:"steps"`
	Suggests  []string          `json:"suggests,omitempty"`
}

// SkillResolver loads a skill by name.
type SkillResolver func(name string) (*Skill, error)

// ComposedRunResult captures dependency workflow execution.
type ComposedRunResult struct {
	Plan    CompositionPlan `json:"plan"`
	Results []RunResult     `json:"results"`
	Success bool            `json:"success"`
	Summary string          `json:"summary"`
}

func (s *Skill) WorkflowDependencies() DependencyWorkflow {
	wf := DependencyWorkflow{}
	if s == nil {
		return wf
	}
	if s.DependencyWorkflow != nil {
		wf = *s.DependencyWorkflow
	}
	// Backward-compatible plain dependencies are treated as required dependencies.
	if len(s.Dependencies) > 0 {
		wf.Requires = appendUnique(wf.Requires, s.Dependencies...)
	}
	return wf
}

func (s *Skill) CompositionPlan() CompositionPlan {
	wf := s.WorkflowDependencies()
	plan := CompositionPlan{MainSkill: s.Name, Suggests: append([]string(nil), wf.Suggests...)}
	for _, name := range wf.Requires {
		plan.Steps = append(plan.Steps, CompositionStep{Kind: "requires", SkillName: name, Blocking: true})
	}
	for _, name := range wf.Precheck {
		plan.Steps = append(plan.Steps, CompositionStep{Kind: "precheck", SkillName: name, Blocking: true})
	}
	plan.Steps = append(plan.Steps, CompositionStep{Kind: "main", SkillName: s.Name, Blocking: true})
	for _, name := range wf.Postcheck {
		plan.Steps = append(plan.Steps, CompositionStep{Kind: "postcheck", SkillName: name, Blocking: false})
	}
	return plan
}

func (s *Skill) HasWorkflowComposition() bool {
	wf := s.WorkflowDependencies()
	return len(wf.Requires)+len(wf.Suggests)+len(wf.Precheck)+len(wf.Postcheck) > 0
}

// RunComposed executes required dependencies, prechecks, main skill, and postchecks.
// Suggestion dependencies are reported in the plan but never block or auto-run.
func (s *Skill) RunComposed(resolver SkillResolver, executor StepExecutor) (*ComposedRunResult, error) {
	plan := s.CompositionPlan()
	out := &ComposedRunResult{Plan: plan, Success: true}
	for _, step := range plan.Steps {
		var sk *Skill
		if step.Kind == "main" {
			sk = s
		} else {
			loaded, err := resolver(step.SkillName)
			if err != nil {
				out.Success = false
				out.Summary = fmt.Sprintf("%s dependency '%s' tidak ditemukan: %v", step.Kind, step.SkillName, err)
				return out, nil
			}
			sk = loaded
		}
		res, err := sk.Run(executor)
		if err != nil {
			return nil, err
		}
		out.Results = append(out.Results, *res)
		if !res.Success {
			out.Success = false
			out.Summary = fmt.Sprintf("workflow berhenti: %s '%s' gagal", step.Kind, step.SkillName)
			return out, nil
		}
	}
	out.Summary = "workflow berhasil dieksekusi"
	return out, nil
}

func appendUnique(base []string, values ...string) []string {
	seen := map[string]bool{}
	for _, v := range base {
		seen[v] = true
	}
	for _, v := range values {
		if v != "" && !seen[v] {
			base = append(base, v)
			seen[v] = true
		}
	}
	return base
}
