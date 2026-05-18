package browser

import "time"

// Task describes a browser subagent run derived from a natural language prompt.
type Task struct {
	Prompt   string
	URL      string
	Viewport Viewport
	Steps    []Step
}

type Viewport struct {
	Name   string
	Width  int
	Height int
}

type Step struct {
	Action string
	Target string
	Value  string
	Name   string
}

type StepResult struct {
	Step   Step
	Status string
	Error  string
}

type Result struct {
	ID             string
	Prompt         string
	URL            string
	ArtifactDir    string
	ReportPath     string
	ScreenshotPath string
	Status         string
	StartedAt      time.Time
	FinishedAt     time.Time
	Steps          []StepResult
	ConsoleErrors  []string
	NetworkErrors  []string
}

type Options struct {
	ArtifactRoot string
	Headful      bool
	Timeout      time.Duration
}
