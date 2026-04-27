// Package cognitive re-exports the cognitive schema validator for public use.
package cognitive

import (
	"github.com/gede-cahya/Smara-CLI/internal/cognitive"
)

// Re-export types
type SchemaType = cognitive.SchemaType
type PropertySchema = cognitive.PropertySchema
type ToolSchema = cognitive.ToolSchema
type ValidationResult = cognitive.ValidationResult
type Validator = cognitive.Validator

// Re-export constants
const (
	TypeString  = cognitive.TypeString
	TypeNumber  = cognitive.TypeNumber
	TypeInteger = cognitive.TypeInteger
	TypeBoolean = cognitive.TypeBoolean
	TypeArray   = cognitive.TypeArray
	TypeObject  = cognitive.TypeObject
	TypeNull    = cognitive.TypeNull
)

// Re-export constructors and functions
var NewValidator = cognitive.NewValidator
var ConvertToolFunction = cognitive.ConvertToolFunction
