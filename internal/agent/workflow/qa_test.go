package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseQAResult_PASS(t *testing.T) {
	output := `
Status: PASS
Score: 95
Rekomendasi: All good
`
	result := ParseQAResult(output)
	assert.Equal(t, "PASS", result.Status)
	assert.Equal(t, 95, result.Score)
	assert.Contains(t, result.Report, "Status: PASS")
	assert.Empty(t, result.Issues)
}

func TestParseQAResult_FAIL(t *testing.T) {
	output := `
Status: FAIL
Score: 30
Issues:
- API contract mismatch between backend and frontend
- Schema error: missing auth endpoint
Rekomendasi: Fix integration
`
	result := ParseQAResult(output)
	assert.Equal(t, "FAIL", result.Status)
	assert.Equal(t, 30, result.Score)
	assert.Len(t, result.Issues, 2)
	assert.Contains(t, result.Issues, "API contract mismatch between backend and frontend")
	assert.Contains(t, result.Issues, "Schema error: missing auth endpoint")
}

func TestParseQAResult_Pending(t *testing.T) {
	output := "Some neutral text about the weather"
	result := ParseQAResult(output)
	assert.Equal(t, "PENDING", result.Status)
	assert.Equal(t, 0, result.Score)
}

func TestQAResult_Struct(t *testing.T) {
	qr := QAResult{
		Status: "PASS",
		Report: "All tests passed",
		Issues: []string{},
		Score:  100,
	}
	assert.Equal(t, "PASS", qr.Status)
	assert.Equal(t, "All tests passed", qr.Report)
	assert.Empty(t, qr.Issues)
	assert.Equal(t, 100, qr.Score)
}
