package cognitive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypeConstants(t *testing.T) {
	assert.Equal(t, "string", string(TypeString))
	assert.Equal(t, "number", string(TypeNumber))
	assert.Equal(t, "integer", string(TypeInteger))
	assert.Equal(t, "boolean", string(TypeBoolean))
	assert.Equal(t, "array", string(TypeArray))
	assert.Equal(t, "object", string(TypeObject))
	assert.Equal(t, "null", string(TypeNull))
}

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	assert.NotNil(t, v)
}

func TestPropertySchema_Struct(t *testing.T) {
	ps := PropertySchema{
		Type:        TypeString,
		Description: "A name",
		Required:    true,
	}
	assert.Equal(t, TypeString, ps.Type)
	assert.Equal(t, "A name", ps.Description)
	assert.True(t, ps.Required)
}

func TestToolSchema_Struct(t *testing.T) {
	ts := ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"input": {Type: TypeString, Description: "Input text", Required: true},
		},
	}
	assert.Equal(t, "test_tool", ts.Name)
	assert.Equal(t, TypeObject, ts.Type)
	assert.Len(t, ts.Properties, 1)
	assert.Contains(t, ts.Properties, "input")
}
