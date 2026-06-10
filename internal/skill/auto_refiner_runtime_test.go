package skill

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestAutoApplyRefinementStoresProposalWhenAutoApplyDisabled(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	tracker, err := NewExecutionTracker(db)
	require.NoError(t, err)

	original := &Skill{
		Name:        "runtime-refine",
		Description: "Original",
		Version:     1,
		Steps:       []Step{{Tool: "run_command", Args: map[string]interface{}{"command": "false"}}},
	}
	proposal := `{"name":"runtime-refine","description":"Improved","version":2,"steps":[{"tool":"run_command","args":{"command":"true"}}]}`
	refined, err := AutoApplyRefinement(proposal, original, tracker, RefinerConfig{AutoApply: false})
	require.NoError(t, err)
	assert.Equal(t, 2, refined.Version)

	items, err := tracker.GetImprovements("runtime-refine", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.False(t, items[0].Applied)
	assert.Contains(t, items[0].ProposedJSON, "Improved")
}
