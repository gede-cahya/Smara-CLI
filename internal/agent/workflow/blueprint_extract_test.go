package workflow

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractJSON_TrimsTrailingProse(t *testing.T) {
	raw := `Berikut blueprint:
{"project_name":"Demo","agents":[{"role":"backend","tasks":[{"id":"t1","description":"do it"}]}]}
Selesai.`

	extracted := extractJSON(raw)
	var bp Blueprint
	require.NoError(t, json.Unmarshal([]byte(extracted), &bp))
	assert.Equal(t, "Demo", bp.ProjectName)
	assert.Len(t, bp.Agents, 1)
}

func TestExtractJSON_HandlesNestedBracesInStrings(t *testing.T) {
	raw := `{"project_name":"Demo {x}","agents":[{"role":"qa","tasks":[{"id":"t1","description":"check quote \\\" } still string"}]}]} trailing`

	extracted := extractJSON(raw)
	var bp Blueprint
	require.NoError(t, json.Unmarshal([]byte(extracted), &bp))
	assert.Equal(t, "Demo {x}", bp.ProjectName)
}
