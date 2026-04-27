// Package cognitive provides strict JSON Schema validation for tool function calling,
// enforcing the Cognitive Layer's precision requirement (Hermes concept).
package cognitive

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
)

// SchemaType represents a JSON Schema type.
type SchemaType string

const (
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
	TypeBoolean SchemaType = "boolean"
	TypeArray   SchemaType = "array"
	TypeObject  SchemaType = "object"
	TypeNull    SchemaType = "null"
)

// PropertySchema defines the schema for a single property.
type PropertySchema struct {
	Type        SchemaType                `json:"type"`
	Description string                    `json:"description,omitempty"`
	Enum        []interface{}             `json:"enum,omitempty"`
	Pattern     string                    `json:"pattern,omitempty"`
	MinLength   *int                      `json:"minLength,omitempty"`
	MaxLength   *int                      `json:"maxLength,omitempty"`
	Minimum     *float64                  `json:"minimum,omitempty"`
	Maximum     *float64                  `json:"maximum,omitempty"`
	Required    bool                      `json:"-"` // set by parent required list
	Items       *PropertySchema           `json:"items,omitempty"`
	Properties  map[string]PropertySchema `json:"properties,omitempty"`
}

// ToolSchema defines the strict schema for a tool's input parameters.
type ToolSchema struct {
	Name       string                    `json:"name"`
	Type       SchemaType                `json:"type"`
	Properties map[string]PropertySchema `json:"properties"`
	Required   []string                  `json:"required"`
}

// ValidationResult contains validation errors.
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// Validator performs strict schema validation.
type Validator struct {
	schemas map[string]ToolSchema
}

// NewValidator creates a new strict validator.
func NewValidator() *Validator {
	return &Validator{
		schemas: make(map[string]ToolSchema),
	}
}

// RegisterTool registers a strict schema for a tool.
func (v *Validator) RegisterTool(schema ToolSchema) {
	v.schemas[schema.Name] = schema
}

// Validate validates tool arguments against the registered schema.
func (v *Validator) Validate(toolName string, args map[string]interface{}) ValidationResult {
	schema, ok := v.schemas[toolName]
	if !ok {
		return ValidationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("schema for tool '%s' not registered", toolName)},
		}
	}

	var errors []string

	// Check required fields
	for _, req := range schema.Required {
		if _, ok := args[req]; !ok {
			errors = append(errors, fmt.Sprintf("missing required field: %s", req))
		}
	}

	// Validate each argument
	for key, value := range args {
		prop, ok := schema.Properties[key]
		if !ok {
			errors = append(errors, fmt.Sprintf("unknown field: %s", key))
			continue
		}
		if errs := validateValue(key, value, prop); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

func validateValue(path string, value interface{}, schema PropertySchema) []string {
	var errors []string

	// Type validation
	if !isValidType(value, schema.Type) {
		errors = append(errors, fmt.Sprintf("%s: expected type %s, got %T", path, schema.Type, value))
		return errors
	}

	// String validations
	if str, ok := value.(string); ok && schema.Type == TypeString {
		if schema.MinLength != nil && len(str) < *schema.MinLength {
			errors = append(errors, fmt.Sprintf("%s: string length %d < minimum %d", path, len(str), *schema.MinLength))
		}
		if schema.MaxLength != nil && len(str) > *schema.MaxLength {
			errors = append(errors, fmt.Sprintf("%s: string length %d > maximum %d", path, len(str), *schema.MaxLength))
		}
		if schema.Pattern != "" {
			re, err := regexp.Compile(schema.Pattern)
			if err == nil && !re.MatchString(str) {
				errors = append(errors, fmt.Sprintf("%s: value '%s' does not match pattern '%s'", path, str, schema.Pattern))
			}
		}
		if len(schema.Enum) > 0 {
			found := false
			for _, e := range schema.Enum {
				if e == str {
					found = true
					break
				}
			}
			if !found {
				errors = append(errors, fmt.Sprintf("%s: value '%s' not in enum %v", path, str, schema.Enum))
			}
		}
	}

	// Number validations
	if schema.Type == TypeNumber || schema.Type == TypeInteger {
		var num float64
		switch v := value.(type) {
		case float64:
			num = v
		case int:
			num = float64(v)
		case int64:
			num = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				num = f
			} else {
				errors = append(errors, fmt.Sprintf("%s: cannot parse number from '%s'", path, v))
				return errors
			}
		default:
			errors = append(errors, fmt.Sprintf("%s: expected number, got %T", path, value))
			return errors
		}

		if schema.Minimum != nil && num < *schema.Minimum {
			errors = append(errors, fmt.Sprintf("%s: value %v < minimum %v", path, num, *schema.Minimum))
		}
		if schema.Maximum != nil && num > *schema.Maximum {
			errors = append(errors, fmt.Sprintf("%s: value %v > maximum %v", path, num, *schema.Maximum))
		}
		if schema.Type == TypeInteger && num != float64(int64(num)) {
			errors = append(errors, fmt.Sprintf("%s: expected integer, got %v", path, num))
		}
	}

	// Array validations
	if schema.Type == TypeArray {
		arr := reflect.ValueOf(value)
		if arr.Kind() != reflect.Slice && arr.Kind() != reflect.Array {
			errors = append(errors, fmt.Sprintf("%s: expected array", path))
			return errors
		}
		if schema.Items != nil {
			for i := 0; i < arr.Len(); i++ {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				item := arr.Index(i).Interface()
				if errs := validateValue(itemPath, item, *schema.Items); len(errs) > 0 {
					errors = append(errors, errs...)
				}
			}
		}
	}

	// Object validations
	if schema.Type == TypeObject {
		if m, ok := value.(map[string]interface{}); ok && schema.Properties != nil {
			for k, v := range m {
				if prop, ok := schema.Properties[k]; ok {
					if errs := validateValue(fmt.Sprintf("%s.%s", path, k), v, prop); len(errs) > 0 {
						errors = append(errors, errs...)
					}
				}
			}
		}
	}

	return errors
}

func isValidType(value interface{}, expected SchemaType) bool {
	if value == nil {
		return expected == TypeNull
	}

	switch expected {
	case TypeString:
		_, ok := value.(string)
		return ok
	case TypeNumber:
		switch value.(type) {
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		}
		return false
	case TypeInteger:
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		case float64:
			v := value.(float64)
			return v == float64(int64(v))
		}
		return false
	case TypeBoolean:
		_, ok := value.(bool)
		return ok
	case TypeArray:
		kind := reflect.TypeOf(value).Kind()
		return kind == reflect.Slice || kind == reflect.Array
	case TypeObject:
		_, ok := value.(map[string]interface{})
		return ok
	case TypeNull:
		return value == nil
	default:
		return true
	}
}

// ConvertToolFunction converts an llm.ToolFunction to a strict ToolSchema.
func ConvertToolFunction(name string, params map[string]interface{}) ToolSchema {
	schema := ToolSchema{
		Name:       name,
		Type:       TypeObject,
		Properties: make(map[string]PropertySchema),
	}

	if props, ok := params["properties"].(map[string]interface{}); ok {
		for key, val := range props {
			if propMap, ok := val.(map[string]interface{}); ok {
				schema.Properties[key] = convertProperty(propMap)
			}
		}
	}

	if req, ok := params["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}

	return schema
}

func convertProperty(m map[string]interface{}) PropertySchema {
	prop := PropertySchema{}

	if t, ok := m["type"].(string); ok {
		prop.Type = SchemaType(t)
	}
	if d, ok := m["description"].(string); ok {
		prop.Description = d
	}
	if p, ok := m["pattern"].(string); ok {
		prop.Pattern = p
	}
	if min, ok := m["minimum"].(float64); ok {
		prop.Minimum = &min
	}
	if max, ok := m["maximum"].(float64); ok {
		prop.Maximum = &max
	}
	if minLen, ok := m["minLength"].(float64); ok {
		i := int(minLen)
		prop.MinLength = &i
	}
	if maxLen, ok := m["maxLength"].(float64); ok {
		i := int(maxLen)
		prop.MaxLength = &i
	}
	if enum, ok := m["enum"].([]interface{}); ok {
		prop.Enum = enum
	}

	return prop
}
