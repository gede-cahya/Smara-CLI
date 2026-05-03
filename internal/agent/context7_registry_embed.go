package agent

import (
	_ "embed"
	"encoding/json"
)

//go:embed context7_registry.json
var context7RegistryEmbed []byte

// loadEmbeddedContext7Registry attempts to parse the embedded registry index.
func loadEmbeddedContext7Registry() (*Context7RegistryManifest, error) {
	var manifest Context7RegistryManifest
	if err := json.Unmarshal(context7RegistryEmbed, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}
