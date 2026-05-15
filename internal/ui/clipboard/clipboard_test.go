package clipboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrite(t *testing.T) {
	// Write emits OSC 52 to stdout and tries native clipboard. We can't
	// intercept either reliably in tests, but it must not panic and must
	// not return an error when at least one path succeeds.
	assert.NoError(t, Write("hello"))
}

func TestRead(t *testing.T) {
	// Read shells out to xclip / wl-paste / pbpaste / clip.exe. In CI
	// environments these may or may not be available — both outcomes are
	// acceptable. The contract we care about is that Read does not panic
	// and does not block the calling goroutine on stdin.
	_, _ = Read()
}
