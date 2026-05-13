package skill

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFingerprint_SameShape_SameHash(t *testing.T) {
	t1 := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "a", "command": "uptime"}},
		{Tool: "run_command", Args: map[string]interface{}{"command": "ls"}},
	}}
	t2 := ExecutionTrace{Steps: []TraceStep{
		// Same shape but different values
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "b", "command": "free -h"}},
		{Tool: "run_command", Args: map[string]interface{}{"command": "whoami"}},
	}}
	assert.Equal(t, t1.Fingerprint(), t2.Fingerprint(),
		"different arg values but same keys+tools should hash identically")
}

func TestFingerprint_DifferentTools_DifferentHash(t *testing.T) {
	t1 := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "a", "command": "ls"}},
	}}
	t2 := ExecutionTrace{Steps: []TraceStep{
		{Tool: "run_command", Args: map[string]interface{}{"command": "ls"}},
	}}
	assert.NotEqual(t, t1.Fingerprint(), t2.Fingerprint())
}

func TestFingerprint_DifferentOrder_DifferentHash(t *testing.T) {
	t1 := ExecutionTrace{Steps: []TraceStep{
		{Tool: "run_command", Args: map[string]interface{}{"command": "a"}},
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "h"}},
	}}
	t2 := ExecutionTrace{Steps: []TraceStep{
		{Tool: "ssh_exec", Args: map[string]interface{}{"host": "h"}},
		{Tool: "run_command", Args: map[string]interface{}{"command": "a"}},
	}}
	assert.NotEqual(t, t1.Fingerprint(), t2.Fingerprint())
}

func TestRecordTrace_IncrementsAndCrosses(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, EnsureAutoDetectTable(db))

	tr := ExecutionTrace{
		PromptText: "cek servis vps",
		Steps: []TraceStep{
			{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps-cahya", "command": "systemctl status"}},
			{Tool: "ssh_exec", Args: map[string]interface{}{"host": "vps-cahya", "command": "pm2 ls"}},
		},
	}

	rec, crossed, err := RecordTrace(db, tr, 3)
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, 1, rec.Count)
	assert.False(t, crossed)

	rec, crossed, err = RecordTrace(db, tr, 3)
	require.NoError(t, err)
	assert.Equal(t, 2, rec.Count)
	assert.False(t, crossed)

	// Third observation hits threshold — crossed fires exactly once.
	rec, crossed, err = RecordTrace(db, tr, 3)
	require.NoError(t, err)
	assert.Equal(t, 3, rec.Count)
	assert.True(t, crossed, "should fire crossing event at threshold")

	// Fourth observation should not re-fire.
	rec, crossed, err = RecordTrace(db, tr, 3)
	require.NoError(t, err)
	assert.Equal(t, 4, rec.Count)
	assert.False(t, crossed, "should not re-fire after threshold crossed")
}

func TestRecordTrace_TooShortSkipped(t *testing.T) {
	db := newMemDB(t)

	tr := ExecutionTrace{
		Steps: []TraceStep{
			{Tool: "run_command", Args: map[string]interface{}{"command": "ls"}},
		},
	}
	rec, crossed, err := RecordTrace(db, tr, 2)
	require.NoError(t, err)
	assert.Nil(t, rec, "single-step traces are not tracked")
	assert.False(t, crossed)
}

func TestMarkPatternCaptured_StopsCrossingReFire(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, EnsureAutoDetectTable(db))

	tr := ExecutionTrace{
		Steps: []TraceStep{
			{Tool: "run_command", Args: map[string]interface{}{"command": "a"}},
			{Tool: "run_command", Args: map[string]interface{}{"command": "b"}},
		},
	}
	for i := 0; i < 3; i++ {
		_, _, _ = RecordTrace(db, tr, 3)
	}

	// Mark captured
	require.NoError(t, MarkPatternCaptured(db, tr.Fingerprint(), "my-skill"))

	// Next observations should not cross again even with threshold=3 or lower.
	_, crossed, err := RecordTrace(db, tr, 3)
	require.NoError(t, err)
	assert.False(t, crossed, "captured pattern should not re-trigger")
}

func TestSuggestSkillName(t *testing.T) {
	tr := ExecutionTrace{
		Steps: []TraceStep{
			{Tool: "ssh_exec"},
			{Tool: "ssh_exec"},
			{Tool: "run_command"},
		},
	}
	name := SuggestSkillName(tr)
	assert.Contains(t, name, "auto-ssh-exec-run-command")
	assert.Regexp(t, `^auto-[a-z-]+-[0-9a-f]{6}$`, name)
}
