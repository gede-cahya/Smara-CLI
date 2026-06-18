package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// init wires the agent-side semantic validator into the skill refiner so that
// auto-refine proposals are rejected when a step's arguments violate the
// builtin tool schema (unknown enum values, missing required params). Without
// this, the refiner only checks the structural Skill schema and lets the LLM
// guess invalid enum values that fail at run time on every execution.
func init() {
	skill.StepSemanticValidator = validateStepsAgainstBuiltins
}

// toolSchema captures just the parts of a builtin tool's JSON schema that the
// semantic validator needs to check skill step arguments.
type toolSchema struct {
	required []string
	enums    map[string][]string // property name -> allowed values
}

func builtinToolSchemas() map[string]toolSchema {
	schemas := make(map[string]toolSchema)
	for _, t := range allBuiltinTools() {
		schema := toolSchema{enums: map[string][]string{}}
		if req, ok := t.Parameters["required"].([]string); ok {
			schema.required = req
		}
		if props, ok := t.Parameters["properties"].(map[string]interface{}); ok {
			for name, raw := range props {
				prop, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if enum, ok := prop["enum"].([]string); ok && len(enum) > 0 {
					schema.enums[name] = enum
				}
			}
		}
		schemas[t.Name] = schema
	}
	return schemas
}

func validateStepsAgainstBuiltins(steps []skill.Step) error {
	schemas := builtinToolSchemas()
	for i, st := range steps {
		schema, known := schemas[st.Tool]
		if !known {
			// Non-builtin tools (MCP, dynamic) can't be checked here; skip.
			continue
		}
		for _, req := range schema.required {
			if v, ok := st.Args[req]; !ok || isBlank(v) {
				return fmt.Errorf("step %d (%s): argumen wajib '%s' kosong atau hilang", i+1, st.Tool, req)
			}
		}
		for name, allowed := range schema.enums {
			raw, ok := st.Args[name]
			if !ok {
				continue
			}
			val, ok := raw.(string)
			if !ok {
				return fmt.Errorf("step %d (%s): argumen '%s' harus string", i+1, st.Tool, name)
			}
			if !contains(allowed, val) {
				return fmt.Errorf("step %d (%s): nilai '%s'='%s' tidak valid, harus salah satu dari [%s]",
					i+1, st.Tool, name, val, strings.Join(sortedCopy(allowed), ", "))
			}
		}
	}
	return nil
}

func isBlank(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

func contains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
