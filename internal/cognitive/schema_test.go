package cognitive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	assert.NotNil(t, v)
	assert.NotNil(t, v.schemas)
	assert.Empty(t, v.schemas)
}

func TestValidator_RegisterTool(t *testing.T) {
	v := NewValidator()
	schema := ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"name": {Type: TypeString},
		},
		Required: []string{"name"},
	}
	v.RegisterTool(schema)
	assert.Contains(t, v.schemas, "test_tool")
}

func TestValidator_Validate_RequiredFieldMissing(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"name": {Type: TypeString},
			"age":  {Type: TypeInteger},
		},
		Required: []string{"name"},
	})

	result := v.Validate("test_tool", map[string]interface{}{
		"age": 30,
	})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "missing required field: name")
}

func TestValidator_Validate_UnknownField(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"name": {Type: TypeString},
		},
		Required: []string{"name"},
	})

	result := v.Validate("test_tool", map[string]interface{}{
		"name":  "Alice",
		"extra": "value",
	})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "unknown field: extra")
}

func TestValidator_Validate_SchemaNotRegistered(t *testing.T) {
	v := NewValidator()
	result := v.Validate("unknown_tool", map[string]interface{}{})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "schema for tool 'unknown_tool' not registered")
}

func TestValidator_Validate_TypeString(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"name": {Type: TypeString},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"name": "Alice"})
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)

	result = v.Validate("test_tool", map[string]interface{}{"name": 123})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "name: expected type string, got int")
}

func TestValidator_Validate_TypeNumber(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"value": {Type: TypeNumber},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"value": 3.14})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"value": 42})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"value": "not a number"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "value: expected type number, got string")
}

func TestValidator_Validate_TypeInteger(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"count": {Type: TypeInteger},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"count": 42})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"count": 3.14})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "count: expected type integer, got float64")
}

func TestValidator_Validate_TypeBoolean(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"active": {Type: TypeBoolean},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"active": true})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"active": "true"})
	assert.False(t, result.Valid)
}

func TestValidator_Validate_TypeNull(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"optional": {Type: TypeNull},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"optional": nil})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"optional": "value"})
	assert.False(t, result.Valid)
}

func TestValidator_Validate_StringMinLength(t *testing.T) {
	minLen := 3
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"code": {Type: TypeString, MinLength: &minLen},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"code": "AB"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "code: string length 2 < minimum 3")

	result = v.Validate("test_tool", map[string]interface{}{"code": "ABC"})
	assert.True(t, result.Valid)
}

func TestValidator_Validate_StringMaxLength(t *testing.T) {
	maxLen := 5
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"code": {Type: TypeString, MaxLength: &maxLen},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"code": "ABCDEF"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "code: string length 6 > maximum 5")

	result = v.Validate("test_tool", map[string]interface{}{"code": "ABC"})
	assert.True(t, result.Valid)
}

func TestValidator_Validate_StringPattern(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"email": {Type: TypeString, Pattern: `^\S+@\S+\.\S+$`},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"email": "test@example.com"})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"email": "invalid"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "email: value 'invalid' does not match pattern '^\\S+@\\S+\\.\\S+$'")
}

func TestValidator_Validate_StringEnum(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"status": {Type: TypeString, Enum: []interface{}{"active", "inactive"}},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"status": "active"})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"status": "unknown"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "status: value 'unknown' not in enum [active inactive]")
}

func TestValidator_Validate_NumberMinimum(t *testing.T) {
	min := 0.0
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"score": {Type: TypeNumber, Minimum: &min},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"score": -1})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "score: value -1 < minimum 0")

	result = v.Validate("test_tool", map[string]interface{}{"score": 0})
	assert.True(t, result.Valid)
}

func TestValidator_Validate_NumberMaximum(t *testing.T) {
	max := 100.0
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"score": {Type: TypeNumber, Maximum: &max},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"score": 101})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "score: value 101 > maximum 100")

	result = v.Validate("test_tool", map[string]interface{}{"score": 100})
	assert.True(t, result.Valid)
}

func TestValidator_Validate_NumberFromString(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"value": {Type: TypeNumber},
		},
	})

	// String values are rejected at the type-check level, not parsed
	result := v.Validate("test_tool", map[string]interface{}{"value": "3.14"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "value: expected type number, got string")

	result = v.Validate("test_tool", map[string]interface{}{"value": "not a number"})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "value: expected type number, got string")
}

func TestValidator_Validate_ArrayWithItems(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"tags": {
				Type:  TypeArray,
				Items: &PropertySchema{Type: TypeString},
			},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{"tags": []string{"a", "b"}})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{"tags": []interface{}{"a", 123}})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "tags[1]: expected type string, got int")
}

func TestValidator_Validate_ObjectWithProperties(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"config": {
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"host": {Type: TypeString},
					"port": {Type: TypeInteger},
				},
			},
		},
	})

	result := v.Validate("test_tool", map[string]interface{}{
		"config": map[string]interface{}{
			"host": "localhost",
			"port": 8080,
		},
	})
	assert.True(t, result.Valid)

	result = v.Validate("test_tool", map[string]interface{}{
		"config": map[string]interface{}{
			"host": "localhost",
			"port": "8080",
		},
	})
	assert.False(t, result.Valid)
	assert.Contains(t, result.Errors, "config.port: expected type integer, got string")
}

func TestValidator_Validate_MultipleErrors(t *testing.T) {
	v := NewValidator()
	v.RegisterTool(ToolSchema{
		Name: "test_tool",
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"name": {Type: TypeString},
			"age":  {Type: TypeInteger},
		},
		Required: []string{"name", "age"},
	})

	result := v.Validate("test_tool", map[string]interface{}{})
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 2)
	assert.Contains(t, result.Errors, "missing required field: name")
	assert.Contains(t, result.Errors, "missing required field: age")
}

func TestConvertToolFunction(t *testing.T) {
	params := map[string]interface{}{
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "file path",
			},
			"content": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"path"},
	}

	schema := ConvertToolFunction("write_file", params)
	assert.Equal(t, "write_file", schema.Name)
	assert.Equal(t, TypeObject, schema.Type)
	assert.Contains(t, schema.Properties, "path")
	assert.Contains(t, schema.Properties, "content")
	assert.Equal(t, []string{"path"}, schema.Required)
	assert.Equal(t, TypeString, schema.Properties["path"].Type)
	assert.Equal(t, "file path", schema.Properties["path"].Description)
}

func TestConvertProperty(t *testing.T) {
	m := map[string]interface{}{
		"type":        "string",
		"description": "a field",
		"pattern":     `^\d+$`,
		"minimum":     1.0,
		"maximum":     100.0,
		"minLength":   2.0,
		"maxLength":   10.0,
		"enum":        []interface{}{"a", "b"},
	}

	prop := convertProperty(m)
	assert.Equal(t, TypeString, prop.Type)
	assert.Equal(t, "a field", prop.Description)
	assert.Equal(t, `^\d+$`, prop.Pattern)
	assert.NotNil(t, prop.Minimum)
	assert.Equal(t, 1.0, *prop.Minimum)
	assert.NotNil(t, prop.Maximum)
	assert.Equal(t, 100.0, *prop.Maximum)
	assert.NotNil(t, prop.MinLength)
	assert.Equal(t, 2, *prop.MinLength)
	assert.NotNil(t, prop.MaxLength)
	assert.Equal(t, 10, *prop.MaxLength)
	assert.Equal(t, []interface{}{"a", "b"}, prop.Enum)
}

func TestConvertProperty_EmptyMap(t *testing.T) {
	prop := convertProperty(map[string]interface{}{})
	assert.Equal(t, SchemaType(""), prop.Type)
	assert.Empty(t, prop.Description)
	assert.Nil(t, prop.Minimum)
	assert.Nil(t, prop.Maximum)
	assert.Nil(t, prop.MinLength)
	assert.Nil(t, prop.MaxLength)
	assert.Empty(t, prop.Enum)
}

func TestIsValidType(t *testing.T) {
	assert.True(t, isValidType("hello", TypeString))
	assert.False(t, isValidType(123, TypeString))

	assert.True(t, isValidType(3.14, TypeNumber))
	assert.True(t, isValidType(42, TypeNumber))
	assert.False(t, isValidType("3.14", TypeNumber))

	assert.True(t, isValidType(42, TypeInteger))
	assert.True(t, isValidType(float64(42), TypeInteger))
	assert.False(t, isValidType(3.14, TypeInteger))

	assert.True(t, isValidType(true, TypeBoolean))
	assert.False(t, isValidType("true", TypeBoolean))

	assert.True(t, isValidType([]string{"a"}, TypeArray))
	assert.False(t, isValidType("not array", TypeArray))

	assert.True(t, isValidType(map[string]interface{}{}, TypeObject))
	assert.False(t, isValidType("not object", TypeObject))

	assert.True(t, isValidType(nil, TypeNull))
	assert.False(t, isValidType("not null", TypeNull))
}

func TestValidationResult_Struct(t *testing.T) {
	r := ValidationResult{Valid: true, Errors: []string{}}
	assert.True(t, r.Valid)
	assert.Empty(t, r.Errors)

	r = ValidationResult{Valid: false, Errors: []string{"error1", "error2"}}
	assert.False(t, r.Valid)
	assert.Len(t, r.Errors, 2)
}

func TestToolSchema_Struct(t *testing.T) {
	schema := ToolSchema{
		Name:       "test",
		Type:       TypeObject,
		Properties: map[string]PropertySchema{"x": {Type: TypeString}},
		Required:   []string{"x"},
	}
	assert.Equal(t, "test", schema.Name)
	assert.Equal(t, TypeObject, schema.Type)
	assert.Len(t, schema.Properties, 1)
	assert.Equal(t, []string{"x"}, schema.Required)
}

func TestPropertySchema_Struct(t *testing.T) {
	minLen := 5
	prop := PropertySchema{
		Type:        TypeString,
		Description: "desc",
		MinLength:   &minLen,
		Required:    true,
		Items:       &PropertySchema{Type: TypeString},
		Properties:  map[string]PropertySchema{"nested": {Type: TypeInteger}},
	}
	assert.Equal(t, TypeString, prop.Type)
	assert.Equal(t, "desc", prop.Description)
	assert.Equal(t, 5, *prop.MinLength)
	assert.True(t, prop.Required)
	assert.NotNil(t, prop.Items)
	assert.Len(t, prop.Properties, 1)
}
