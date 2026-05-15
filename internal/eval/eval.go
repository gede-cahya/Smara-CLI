package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

type Suite struct {
	Name  string     `json:"name"`
	Cases []TestCase `json:"cases"`
}

type TestCase struct {
	Name     string   `json:"name"`
	Prompt   string   `json:"prompt"`
	Contains []string `json:"contains,omitempty"`
}

type Result struct {
	Suite      string       `json:"suite"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Total      int          `json:"total"`
	Passed     int          `json:"passed"`
	Failed     int          `json:"failed"`
	Cases      []CaseResult `json:"cases"`
}

type CaseResult struct {
	Name        string        `json:"name"`
	Passed      bool          `json:"passed"`
	LatencyMs   int64         `json:"latency_ms"`
	TotalTokens int           `json:"total_tokens,omitempty"`
	Error       string        `json:"error,omitempty"`
	Response    string        `json:"response,omitempty"`
	Missing     []string      `json:"missing,omitempty"`
	Duration    time.Duration `json:"-"`
}

func DefaultSuite() Suite {
	return Suite{
		Name: "smoke",
		Cases: []TestCase{{
			Name:     "basic-response",
			Prompt:   "Jawab hanya dengan kata OK.",
			Contains: []string{"OK"},
		}},
	}
}

func LoadSuite(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("gagal baca eval suite: %w", err)
	}
	var suite Suite
	if err := json.Unmarshal(data, &suite); err != nil {
		return Suite{}, fmt.Errorf("gagal parse eval suite: %w", err)
	}
	if suite.Name == "" {
		suite.Name = path
	}
	if len(suite.Cases) == 0 {
		return Suite{}, fmt.Errorf("eval suite tidak punya cases")
	}
	return suite, nil
}

func Run(provider llm.Provider, suite Suite) Result {
	result := Result{Suite: suite.Name, StartedAt: time.Now(), Total: len(suite.Cases)}
	for _, tc := range suite.Cases {
		caseResult := runCase(provider, tc)
		result.Cases = append(result.Cases, caseResult)
		if caseResult.Passed {
			result.Passed++
		} else {
			result.Failed++
		}
	}
	result.FinishedAt = time.Now()
	return result
}

func runCase(provider llm.Provider, tc TestCase) CaseResult {
	started := time.Now()
	res := CaseResult{Name: tc.Name}
	resp, err := provider.Chat([]llm.Message{{Role: llm.RoleUser, Content: tc.Prompt}})
	res.Duration = time.Since(started)
	res.LatencyMs = res.Duration.Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Response = resp.Content
	res.TotalTokens = resp.TotalTokens
	lower := strings.ToLower(resp.Content)
	for _, want := range tc.Contains {
		if !strings.Contains(lower, strings.ToLower(want)) {
			res.Missing = append(res.Missing, want)
		}
	}
	res.Passed = len(res.Missing) == 0
	return res
}
