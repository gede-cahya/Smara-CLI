package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSkill_ToJSON(t *testing.T) {
	s := Skill{Name: "test", Steps: []Step{{Tool: "echo", Args: map[string]interface{}{}}}}
	data, err := s.ToJSON()
	assert.NoError(t, err)
	assert.Contains(t, string(data), "test")
}

func TestFromJSON_Valid(t *testing.T) {
	data := []byte(`{"name":"test","steps":[{"tool":"echo"}]}`)
	s, err := FromJSON(data)
	assert.NoError(t, err)
	assert.Equal(t, "test", s.Name)
	assert.Len(t, s.Steps, 1)
	assert.Equal(t, "echo", s.Steps[0].Tool)
}

func TestFromJSON_Invalid(t *testing.T) {
	_, err := FromJSON([]byte(`{invalid`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestValidate_MissingName(t *testing.T) {
	s := Skill{Steps: []Step{{Tool: "echo"}}}
	assert.EqualError(t, s.Validate(), "skill name is required")
}

func TestValidate_NoSteps(t *testing.T) {
	s := Skill{Name: "test"}
	assert.EqualError(t, s.Validate(), "skill 'test' has no steps")
}

func TestValidate_EmptyTool(t *testing.T) {
	s := Skill{Name: "test", Steps: []Step{{Tool: ""}}}
	assert.EqualError(t, s.Validate(), "skill 'test' step 0 has empty tool")
}

func TestValidate_Ok(t *testing.T) {
	s := Skill{Name: "test", Steps: []Step{{Tool: "echo", Args: map[string]interface{}{"msg": "hi"}}}}
	assert.NoError(t, s.Validate())
}
