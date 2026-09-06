package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// UI authentication modes for AuthConfig.UI.
const (
	// UIAuthAPIKeys guards the web UI with apiKeys, the historical behaviour.
	UIAuthAPIKeys = "apiKeys"
	// UIAuthNone leaves the web UI unauthenticated; a reverse proxy must gate it.
	UIAuthNone = "none"
)

// AuthConfig controls how the web UI and the endpoints only it uses are
// authenticated. Inference endpoints (/v1/*, /models, /upstream/*, /comfyui/*)
// and the operations endpoints always require apiKeys and are unaffected.
type AuthConfig struct {
	// UI selects the web UI mode: apiKeys (default) or none.
	UI string `yaml:"ui"`
}

// UnmarshalYAML validates the mode.
func (a *AuthConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		UI string `yaml:"ui"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	mode := strings.TrimSpace(raw.UI)
	switch mode {
	case "", UIAuthAPIKeys, UIAuthNone:
	default:
		return fmt.Errorf("auth.ui: %q must be %s or %s", raw.UI, UIAuthAPIKeys, UIAuthNone)
	}
	a.UI = mode
	return nil
}

// UIMode returns the effective web UI mode, defaulting to apiKeys.
func (a AuthConfig) UIMode() string {
	if a.UI == "" {
		return UIAuthAPIKeys
	}
	return a.UI
}
