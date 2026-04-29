package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeLayout_Basic(t *testing.T) {
	l := ComputeLayout(120, 40, true, 30)
	assert.Equal(t, 120, l.Width)
	assert.Equal(t, 40, l.Height)
	assert.True(t, l.ShowSidebar)
	assert.Equal(t, 30, l.SidebarW)
	assert.Equal(t, 90, l.ContentW)
	assert.Equal(t, 1, l.HeaderH)
	assert.Equal(t, 1, l.StatusH)
	assert.Equal(t, 3, l.InputH)
}

func TestComputeLayout_Small(t *testing.T) {
	l := ComputeLayout(50, 10, false, 30)
	assert.Equal(t, 60, l.Width)  // minWidth
	assert.Equal(t, 15, l.Height) // minHeight
	assert.False(t, l.ShowSidebar)
	assert.Equal(t, 0, l.SidebarW)
	assert.Equal(t, 60, l.ContentW)
}

func TestComputeLayout_NarrowNoSidebar(t *testing.T) {
	l := ComputeLayout(80, 30, true, 30)
	assert.Equal(t, 0, l.SidebarW)
	assert.Equal(t, 80, l.ContentW)
}
