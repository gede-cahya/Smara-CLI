package skill

import (
	_ "embed"
	"encoding/json"
)

//go:embed builtin_marketplace.json
var builtinMarketplaceJSON []byte

var builtinManifest *RegistryManifest

func init() {
	if len(builtinMarketplaceJSON) > 0 {
		var m RegistryManifest
		if err := json.Unmarshal(builtinMarketplaceJSON, &m); err == nil {
			builtinManifest = &m
		}
	}
}

// BuiltinManifest returns the embedded built-in marketplace registry manifest.
func BuiltinManifest() *RegistryManifest {
	return builtinManifest
}
