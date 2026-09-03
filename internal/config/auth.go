package config

import (
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

// UI authentication modes for AuthConfig.UI.
const (
	// UIAuthAPIKeys guards the web UI with apiKeys, the historical behaviour.
	UIAuthAPIKeys = "apiKeys"
	// UIAuthTrustedHeader accepts only requests carrying AuthConfig.TrustedHeader,
	// set by an authenticating reverse proxy. API keys never open the UI.
	UIAuthTrustedHeader = "trustedHeader"
	// UIAuthNone leaves the web UI unauthenticated; a reverse proxy must gate it.
	UIAuthNone = "none"
)

// AuthConfig controls how the web UI and the endpoints only it uses are
// authenticated. Inference endpoints (/v1/*, /models, /upstream/*, /comfyui/*)
// and the operations endpoints always require apiKeys and are unaffected.
type AuthConfig struct {
	// UI selects the web UI mode: apiKeys (default), trustedHeader or none.
	UI string `yaml:"ui"`

	// TrustedHeader is the request header the proxy sets for authenticated
	// users, typically Remote-User. Required when UI is trustedHeader.
	TrustedHeader string `yaml:"trustedHeader"`

	// TrustedProxies lists the IPs or CIDR ranges allowed to present
	// TrustedHeader. Requests from other sources have the header ignored.
	// Empty accepts the header from any source, which is only safe when
	// nothing but the proxy can reach llama-swap.
	TrustedProxies []string `yaml:"trustedProxies"`

	trustedNets []*net.IPNet
}

// rawAuthConfig is the YAML shape of AuthConfig before validation.
type rawAuthConfig struct {
	UI             string   `yaml:"ui"`
	TrustedHeader  string   `yaml:"trustedHeader"`
	TrustedProxies []string `yaml:"trustedProxies"`
}

// UnmarshalYAML validates the mode, the header name and the proxy list.
func (a *AuthConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw rawAuthConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	mode := strings.TrimSpace(raw.UI)
	switch mode {
	case "", UIAuthAPIKeys, UIAuthTrustedHeader, UIAuthNone:
	default:
		return fmt.Errorf("auth.ui: %q must be one of %s, %s or %s", raw.UI, UIAuthAPIKeys, UIAuthTrustedHeader, UIAuthNone)
	}

	header := strings.TrimSpace(raw.TrustedHeader)
	if strings.ContainsAny(header, " \t:") {
		return fmt.Errorf("auth.trustedHeader: %q is not a valid header name", raw.TrustedHeader)
	}
	nets, err := parseTrustedProxies(raw.TrustedProxies)
	if err != nil {
		return err
	}

	if mode == UIAuthTrustedHeader && header == "" {
		return fmt.Errorf("auth.ui: trustedHeader requires auth.trustedHeader")
	}
	if mode != UIAuthTrustedHeader && (header != "" || len(nets) > 0) {
		return fmt.Errorf("auth.trustedHeader and auth.trustedProxies are only used with auth.ui: %s", UIAuthTrustedHeader)
	}

	a.UI = mode
	a.TrustedHeader = header
	a.TrustedProxies = raw.TrustedProxies
	a.trustedNets = nets
	return nil
}

func parseTrustedProxies(entries []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("auth.trustedProxies: empty entry")
		}
		if _, ipnet, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, ipnet)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("auth.trustedProxies: %q is not an IP address or CIDR range", entry)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return nets, nil
}

// UIMode returns the effective web UI mode, defaulting to apiKeys.
func (a AuthConfig) UIMode() string {
	if a.UI == "" {
		return UIAuthAPIKeys
	}
	return a.UI
}

// IsTrustedSource reports whether a request from ip may present the trusted
// header. With no trustedProxies configured every source is trusted.
func (a AuthConfig) IsTrustedSource(ip net.IP) bool {
	if len(a.trustedNets) == 0 {
		return true
	}
	if ip == nil {
		return false
	}
	for _, n := range a.trustedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
