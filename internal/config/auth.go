package config

import (
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthConfig lets an authenticating reverse proxy (Authelia, oauth2-proxy,
// ...) vouch for web UI users. When TrustedHeader is set, a request carrying
// that header is treated as a UI user: it may use the web UI and its
// endpoints, and it may call inference endpoints without an API key. API keys
// never open the web UI in this mode.
type AuthConfig struct {
	// TrustedHeader is the request header the proxy sets for authenticated
	// users, typically Remote-User. Empty disables trusted header auth.
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
	TrustedHeader  string   `yaml:"trustedHeader"`
	TrustedProxies []string `yaml:"trustedProxies"`
}

// UnmarshalYAML validates the header name and parses each trustedProxies
// entry as a CIDR range or a single IP.
func (a *AuthConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw rawAuthConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	header := strings.TrimSpace(raw.TrustedHeader)
	if strings.ContainsAny(header, " \t:") {
		return fmt.Errorf("auth.trustedHeader: %q is not a valid header name", raw.TrustedHeader)
	}
	nets, err := parseTrustedProxies(raw.TrustedProxies)
	if err != nil {
		return err
	}
	if header == "" && len(nets) > 0 {
		return fmt.Errorf("auth.trustedProxies requires auth.trustedHeader")
	}
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

// Enabled reports whether trusted header auth is configured.
func (a AuthConfig) Enabled() bool {
	return a.TrustedHeader != ""
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
