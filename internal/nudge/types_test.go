package nudge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNudge_Struct(t *testing.T) {
	now := time.Now()
	n := Nudge{
		ID:       1,
		Type:     "info",
		Text:     "test nudge",
		Created:  now,
		ActionID: 123,
	}
	assert.Equal(t, int64(1), n.ID)
	assert.Equal(t, "info", n.Type)
	assert.Equal(t, "test nudge", n.Text)
	assert.Equal(t, now, n.Created)
	assert.Equal(t, int64(123), n.ActionID)
}

func TestNudge_Minimal(t *testing.T) {
	n := Nudge{ID: 2, Type: "warning", Text: "done"}
	assert.Equal(t, int64(2), n.ID)
	assert.Equal(t, "warning", n.Type)
	assert.Equal(t, "done", n.Text)
}
