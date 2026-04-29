package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewHeader(t *testing.T) {
	h := NewHeader(120)
	assert.NotNil(t, h)
	assert.Equal(t, 120, h.width)
	assert.NotNil(t, h.theme)
}

func TestHeader_SetWidth(t *testing.T) {
	h := NewHeader(120)
	h.SetWidth(80)
	assert.Equal(t, 80, h.width)
}

func TestHeader_Render(t *testing.T) {
	h := NewHeader(120)
	result := h.Render("ask", "openai", "gpt-4", false, "", "")
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Smara")
	assert.Contains(t, result, "ASK")
	assert.Contains(t, result, "openai")
	assert.Contains(t, result, "gpt-4")
}

func TestHeader_Render_Processing(t *testing.T) {
	h := NewHeader(120)
	result := h.Render("build", "ollama", "llama3", true, "■", "thinking...")
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "BUILD")
}
