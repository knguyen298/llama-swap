package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Auth_Defaults(t *testing.T) {
	cfg, err := LoadConfigFromReader(strings.NewReader(`apiKeys: ["k"]`))
	assert.NoError(t, err)
	assert.Equal(t, UIAuthAPIKeys, cfg.Auth.UIMode())
	assert.Equal(t, UIAuthAPIKeys, Config{}.Auth.UIMode())
}

func TestConfig_Auth_Modes(t *testing.T) {
	for _, mode := range []string{UIAuthAPIKeys, UIAuthNone} {
		cfg, err := LoadConfigFromReader(strings.NewReader("auth:\n  ui: " + mode + "\n"))
		assert.NoError(t, err)
		assert.Equal(t, mode, cfg.Auth.UIMode())
	}
}

func TestConfig_Auth_Invalid(t *testing.T) {
	_, err := LoadConfigFromReader(strings.NewReader("auth:\n  ui: cookie\n"))
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), `auth.ui: "cookie" must be apiKeys or none`)
	}
}
