package skill

import (
	"fmt"
	"strings"
)

// StepResult holds the outcome of one step.
type StepResult struct {
	Tool   string
	Args   map[string]interface{}
	Output string
	Error  error
}

// RunResult holds the full execution outcome.
type RunResult struct {
	SkillName   string
	StepResults []StepResult
	Success     bool
	Summary     string
}

// StepExecutor is a function that runs a single built-in tool.
type StepExecutor func(toolName string, args map[string]interface{}) (string, error)

// Run executes a skill step-by-step using the provided executor.
func (s *Skill) Run(executor StepExecutor) (*RunResult, error) {
	result := &RunResult{
		SkillName: s.Name,
		Success:   true,
	}

	for i, step := range s.Steps {
		sr := StepResult{
			Tool: step.Tool,
			Args: step.Args,
		}
		out, err := executor(step.Tool, step.Args)
		if err != nil {
			sr.Error = err
			result.Success = false
			result.Summary = fmt.Sprintf("Skill '%s' failed at step %d (%s): %v", s.Name, i+1, step.Tool, err)
			result.StepResults = append(result.StepResults, sr)
			return result, nil
		}
		sr.Output = out
		result.StepResults = append(result.StepResults, sr)
	}

	var summaries []string
	for _, sr := range result.StepResults {
		// Truncate long outputs
		out := sr.Output
		if len(out) > 200 {
			out = out[:200] + "..."
		}
		summaries = append(summaries, fmt.Sprintf("%s: %s", sr.Tool, strings.ReplaceAll(out, "\n", " ")))
	}
	result.Summary = strings.Join(summaries, " | ")
	return result, nil
}

// Summary returns a human-readable description of the skill.
func (s *Skill) Summary() string {
	var steps []string
	for _, st := range s.Steps {
		steps = append(steps, st.Tool)
	}
	return fmt.Sprintf("%s (%d steps: %s)", s.Description, len(s.Steps), strings.Join(steps, " -> "))
}
