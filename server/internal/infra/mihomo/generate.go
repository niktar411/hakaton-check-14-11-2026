// Package mihomo generates and validates the team VPN routing configuration.
package mihomo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// VPNNetwork is the only network routed through the team tunnel.
	VPNNetwork = "10.77.0.0/24"
	// ProviderPath is where the local provider include is mounted on the server.
	ProviderPath = "/etc/mihomo/team-vpn.local.yaml"
)

var allowedFields = map[string]bool{
	"name": true, "type": true, "server": true, "port": true, "ip": true,
	"ipv6": true, "private-key": true, "public-key": true, "pre-shared-key": true,
	"reserved": true, "udp": true, "mtu": true, "remote-dns-resolve": true,
	"persistent-keepalive": true, "amnezia-wg-option": true,
}

// Rules are the only routing rules of the team profile.
func Rules() []string {
	return []string{"IP-CIDR," + VPNNetwork + ",TEAM-VPN,no-resolve", "MATCH,DIRECT"}
}

// ProviderConfig returns the server-side config that reads a local provider.
func ProviderConfig() map[string]any {
	return map[string]any{
		"mixed-port": 7890,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "warning",
		"ipv6":       false,
		"dns":        map[string]any{"enable": false},
		"proxy-providers": map[string]any{
			"team-vpn-local": map[string]any{
				"type": "file", "path": ProviderPath,
				"health-check": map[string]any{"enable": false},
			},
		},
		"proxy-groups": []any{map[string]any{
			"name": "TEAM-VPN", "type": "select", "use": []string{"team-vpn-local"},
		}},
		"rules": Rules(),
	}
}

// Outbound validates one local WireGuard include and forces the team network.
func Outbound(value map[string]any) (map[string]any, error) {
	if value == nil || value["type"] != "wireguard" {
		return nil, errors.New("local outbound must have type wireguard")
	}
	for key := range value {
		if !allowedFields[key] {
			return nil, errors.New("local outbound contains unsupported fields")
		}
	}
	for _, field := range []string{"server", "private-key", "public-key"} {
		text, ok := value[field].(string)
		if !ok || len(text) == 0 {
			return nil, errors.New("local outbound is incomplete")
		}
	}
	port, ok := toInt(value["port"])
	if !ok || port < 1 || port > 65535 {
		return nil, errors.New("local outbound port is invalid")
	}
	result := map[string]any{}
	for key, item := range value {
		result[key] = item
	}
	result["name"] = "team-vpn-wireguard"
	result["allowed-ips"] = []string{VPNNetwork}
	result["udp"] = true
	return result, nil
}

func toInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	case uint64:
		return int(typed), true
	}
	return 0, false
}

// Standalone builds a complete config with a single local node.
func Standalone(local map[string]any) (map[string]any, error) {
	outbound, err := Outbound(local)
	if err != nil {
		return nil, err
	}
	config := ProviderConfig()
	delete(config, "proxy-providers")
	config["proxies"] = []any{outbound}
	config["proxy-groups"] = []any{map[string]any{
		"name": "TEAM-VPN", "type": "select", "proxies": []string{"team-vpn-wireguard"},
	}}
	return config, nil
}

// Generate writes either the provider include or a standalone profile.
func Generate(source string, standalone bool, output string) error {
	if len(source) > 65536 {
		return errors.New("local outbound include is too large")
	}
	var local map[string]any
	if err := yaml.Unmarshal([]byte(source), &local); err != nil {
		return fmt.Errorf("decode local outbound: %w", err)
	}
	var document any
	if standalone {
		config, err := Standalone(local)
		if err != nil {
			return err
		}
		document = config
	} else {
		outbound, err := Outbound(local)
		if err != nil {
			return err
		}
		document = map[string]any{"proxies": []any{outbound}}
	}
	content, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return err
	}
	temporary := output + ".new"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, output)
}
