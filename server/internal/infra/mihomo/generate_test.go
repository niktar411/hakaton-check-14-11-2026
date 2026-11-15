package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func localOutbound(overrides map[string]any) map[string]any {
	value := map[string]any{
		"name": "ignored", "type": "wireguard", "server": "vpn.example", "port": 51820,
		"private-key": "private", "public-key": "public", "ip": "10.77.0.2/32",
		"udp": true, "mtu": 1280,
		"amnezia-wg-option": map[string]any{"version": 3, "jc": 4, "h1": "1", "h2": "2", "h3": "3", "h4": "4"},
	}
	for key, item := range overrides {
		value[key] = item
	}
	return value
}

func TestOutboundForcesTeamNetwork(t *testing.T) {
	result, err := Outbound(localOutbound(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result["name"] != "team-vpn-wireguard" || result["udp"] != true {
		t.Fatalf("result = %v", result)
	}
	if allowed, ok := result["allowed-ips"].([]string); !ok || len(allowed) != 1 || allowed[0] != VPNNetwork {
		t.Fatalf("allowed-ips = %v", result["allowed-ips"])
	}
	if _, ok := result["amnezia-wg-option"]; !ok {
		t.Fatal("amnezia-wg-option was dropped")
	}
}

func TestOutboundRejectsUnsupportedAndIncomplete(t *testing.T) {
	if _, err := Outbound(localOutbound(map[string]any{"dialer-proxy": "MOST"})); err == nil {
		t.Fatal("unsupported field accepted")
	}
	if _, err := Outbound(localOutbound(map[string]any{"type": "vless"})); err == nil {
		t.Fatal("non-wireguard outbound accepted")
	}
	for _, field := range []string{"server", "private-key", "public-key"} {
		if _, err := Outbound(localOutbound(map[string]any{field: ""})); err == nil {
			t.Fatalf("empty %s accepted", field)
		}
	}
	if _, err := Outbound(localOutbound(map[string]any{"port": 0})); err == nil {
		t.Fatal("port 0 accepted")
	}
}

func TestStandaloneRoutesOnlyTeamNetwork(t *testing.T) {
	config, err := Standalone(localOutbound(nil))
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := config["rules"].([]string)
	if len(rules) != 2 || rules[0] != "IP-CIDR,10.77.0.0/24,TEAM-VPN,no-resolve" || rules[1] != "MATCH,DIRECT" {
		t.Fatalf("rules = %v", config["rules"])
	}
	if config["allow-lan"] != false {
		t.Fatal("allow-lan must be false")
	}
	if _, ok := config["proxy-providers"]; ok {
		t.Fatal("standalone profile must not use providers")
	}
}

func TestGenerateWritesProfile(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "local.yaml")
	content, _ := yaml.Marshal(localOutbound(nil))
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "out.yaml")
	if err := Generate(string(content), true, output); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "IP-CIDR,10.77.0.0/24,TEAM-VPN,no-resolve") {
		t.Fatalf("generated = %s", generated)
	}
}

func TestRepositoryMihomoConfigInvariants(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "infra", "mihomo", "hackathon.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	rules, _ := config["rules"].([]any)
	if len(rules) != 2 || rules[0] != "IP-CIDR,10.77.0.0/24,TEAM-VPN,no-resolve" || rules[1] != "MATCH,DIRECT" {
		t.Fatalf("rules = %v", rules)
	}
	if config["allow-lan"] != false || config["ipv6"] != false {
		t.Fatalf("unsafe defaults: %v", config)
	}
	if dns, ok := config["dns"].(map[string]any); !ok || dns["enable"] != false {
		t.Fatalf("dns = %v", config["dns"])
	}
	if strings.Contains(strings.ToLower(string(content)), "privatekey") ||
		strings.Contains(strings.ToLower(string(content)), "private-key") {
		t.Fatal("private key text in repository config")
	}
	providers, _ := config["proxy-providers"].(map[string]any)
	for _, item := range providers {
		provider, _ := item.(map[string]any)
		if provider["type"] != "file" {
			t.Fatalf("provider = %v", provider)
		}
	}
}
