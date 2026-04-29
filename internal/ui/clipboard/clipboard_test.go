package clipboard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	// Write writes to stdout — can't easily intercept, but we can ensure no panic
	assert.NoError(t, Write("hello"))
}

func TestRead(t *testing.T) {
	// Read will timeout in most test environments without terminal OSC52 support
	_, err := Read()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "not supported"))
}
