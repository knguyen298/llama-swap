package config

import (
	"net"
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

func TestConfig_Auth_Parse(t *testing.T) {
	content := `
auth:
  ui: trustedHeader
  trustedHeader: " Remote-User "
  trustedProxies: ["172.18.0.0/16", "10.0.0.5", "fd00::/8"]
`
	cfg, err := LoadConfigFromReader(strings.NewReader(content))
	assert.NoError(t, err)
	assert.Equal(t, UIAuthTrustedHeader, cfg.Auth.UIMode())
	assert.Equal(t, "Remote-User", cfg.Auth.TrustedHeader)
	assert.Equal(t, []string{"172.18.0.0/16", "10.0.0.5", "fd00::/8"}, cfg.Auth.TrustedProxies)

	for _, ip := range []string{"172.18.4.9", "10.0.0.5", "fd00::1"} {
		assert.True(t, cfg.Auth.IsTrustedSource(net.ParseIP(ip)), ip)
	}
	for _, ip := range []string{"172.19.0.1", "10.0.0.6", "127.0.0.1"} {
		assert.False(t, cfg.Auth.IsTrustedSource(net.ParseIP(ip)), ip)
	}
	assert.False(t, cfg.Auth.IsTrustedSource(nil))
}

func TestConfig_Auth_NoneMode(t *testing.T) {
	cfg, err := LoadConfigFromReader(strings.NewReader("auth:\n  ui: none\n"))
	assert.NoError(t, err)
	assert.Equal(t, UIAuthNone, cfg.Auth.UIMode())
}

func TestConfig_Auth_EmptyProxiesTrustsEveryone(t *testing.T) {
	cfg, err := LoadConfigFromReader(strings.NewReader("auth:\n  ui: trustedHeader\n  trustedHeader: Remote-User\n"))
	assert.NoError(t, err)
	assert.True(t, cfg.Auth.IsTrustedSource(net.ParseIP("203.0.113.7")))
}

func TestConfig_Auth_Invalid(t *testing.T) {
	tests := []struct {
		name, content, expectedErr string
	}{
		{"unknown mode", "auth:\n  ui: cookie\n",
			`auth.ui: "cookie" must be one of apiKeys, trustedHeader or none`},
		{"trustedHeader mode without header", "auth:\n  ui: trustedHeader\n",
			"auth.ui: trustedHeader requires auth.trustedHeader"},
		{"header with default mode", "auth:\n  trustedHeader: Remote-User\n",
			"auth.trustedHeader and auth.trustedProxies are only used with auth.ui: trustedHeader"},
		{"proxies with none mode", "auth:\n  ui: none\n  trustedProxies: [\"10.0.0.0/8\"]\n",
			"auth.trustedHeader and auth.trustedProxies are only used with auth.ui: trustedHeader"},
		{"bad cidr", "auth:\n  ui: trustedHeader\n  trustedHeader: Remote-User\n  trustedProxies: [\"nope\"]\n",
			`auth.trustedProxies: "nope" is not an IP address or CIDR range`},
		{"empty entry", "auth:\n  ui: trustedHeader\n  trustedHeader: Remote-User\n  trustedProxies: [\"\"]\n",
			"auth.trustedProxies: empty entry"},
		{"header with space", "auth:\n  ui: trustedHeader\n  trustedHeader: \"Remote User\"\n",
			`auth.trustedHeader: "Remote User" is not a valid header name`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfigFromReader(strings.NewReader(tt.content))
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tt.expectedErr)
			}
		})
	}
}
