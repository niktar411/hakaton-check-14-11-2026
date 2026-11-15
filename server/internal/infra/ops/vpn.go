package ops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// PeerRecord is the persisted inventory entry of one VPN peer.
type PeerRecord struct {
	Public  string `json:"public"`
	Peer    string `json:"peer"`
	Enabled bool   `json:"enabled"`
}

// PeerInfo is the public inventory view.
type PeerInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Enabled         bool   `json:"enabled"`
	Protected       bool   `json:"protected"`
	LatestHandshake int64  `json:"latestHandshake"`
	ReceivedBytes   int64  `json:"receivedBytes"`
	SentBytes       int64  `json:"sentBytes"`
}

// CommandFunc runs one fixed command. It is injectable for tests.
type CommandFunc func(args []string, input string) (string, error)

// VPNConfig configures the restricted VPN control agent.
type VPNConfig struct {
	ConfigPath string
	Profiles   string
	StatePath  string
}

// VPN owns the peer inventory and transactional config changes.
type VPN struct {
	config VPNConfig
	mu     sync.Mutex
	run    CommandFunc
	names  map[string]string
	addrs  map[string]string
}

var peerNames = map[string]string{
	"nikita": "Никита · тимлид", "egor": "Егор", "leonid": "Леонид", "igor": "Игорь",
}

var peerAddresses = map[string]string{
	"nikita": "10.77.0.2", "egor": "10.77.0.3", "leonid": "10.77.0.4", "igor": "10.77.0.5",
}

// NewVPN builds the VPN controller.
func NewVPN(config VPNConfig) *VPN {
	return &VPN{config: config, run: vpnCommand, names: peerNames, addrs: peerAddresses}
}

// SetCommand replaces the command runner in tests.
func (v *VPN) SetCommand(run CommandFunc) { v.run = run }

func vpnCommand(args []string, input string) (string, error) {
	command := exec.Command(args[0], args[1:]...)
	command.Stdin = strings.NewReader(input)
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func splitPeers(config string) []string {
	parts := strings.Split(config, "[Peer]")
	if len(parts) <= 1 {
		return nil
	}
	return parts[1:]
}

func sectionValues(section string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

// ReplaceField replaces exactly one field in a config section.
func ReplaceField(text, key, value string) (string, error) {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	matches := []int{}
	for index, line := range lines {
		name, _, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(name) == key {
			matches = append(matches, index)
		}
	}
	if len(matches) != 1 {
		return "", errors.New("expected exactly one field")
	}
	lines[matches[0]] = key + " = " + value
	return strings.Join(lines, "\n") + "\n", nil
}

func (v *VPN) stateFile() string { return v.config.StatePath }

func (v *VPN) transactionFile() string {
	return filepath.Join(filepath.Dir(v.config.StatePath), "transaction.json")
}

func (v *VPN) runtimeFile() string {
	return filepath.Join(filepath.Dir(v.config.StatePath), "runtime.conf")
}

func writePrivate(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".new"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}

// LoadState reads or bootstraps the peer inventory.
func (v *VPN) LoadState() (map[string]PeerRecord, error) {
	if content, err := os.ReadFile(v.stateFile()); err == nil {
		records := map[string]PeerRecord{}
		if err := json.Unmarshal(content, &records); err != nil {
			return nil, errors.New("existing peer inventory mismatch")
		}
		return records, nil
	}
	configContent, err := os.ReadFile(v.config.ConfigPath)
	if err != nil {
		return nil, err
	}
	sections := splitPeers(string(configContent))
	records := map[string]PeerRecord{}
	for peerID := range v.names {
		profile, err := os.ReadFile(filepath.Join(v.config.Profiles, peerID+".conf"))
		if err != nil {
			return nil, err
		}
		interfaceSection := strings.Split(string(profile), "[Peer]")[0]
		private := sectionValues(interfaceSection)["PrivateKey"]
		if private == "" {
			return nil, errors.New("profile has no private key")
		}
		public, err := v.run([]string{"awg", "pubkey"}, private)
		if err != nil {
			return nil, err
		}
		matches := []string{}
		for _, section := range sections {
			if sectionValues(section)["PublicKey"] == public {
				matches = append(matches, section)
			}
		}
		if len(matches) != 1 || sectionValues(matches[0])["AllowedIPs"] != v.addrs[peerID]+"/32" {
			return nil, errors.New("existing peer inventory mismatch")
		}
		records[peerID] = PeerRecord{Public: public, Peer: matches[0], Enabled: true}
	}
	content, err := json.Marshal(records)
	if err != nil {
		return nil, err
	}
	if err := writePrivate(v.stateFile(), content); err != nil {
		return nil, err
	}
	return records, nil
}

// ApplyRuntime strips the config and synchronizes the running interface.
func (v *VPN) ApplyRuntime() error {
	stripped, err := v.run([]string{"awg-quick", "strip", v.config.ConfigPath}, "")
	if err != nil {
		return err
	}
	runtime := v.runtimeFile()
	if err := writePrivate(runtime, []byte(stripped+"\n")); err != nil {
		return err
	}
	defer os.Remove(runtime)
	_, err = v.run([]string{"awg", "syncconf", "awg0", runtime}, "")
	return err
}

type transaction struct {
	Config  string `json:"config"`
	State   string `json:"state"`
	Profile string `json:"profile"`
	ID      string `json:"id"`
}

// RecoverTransaction restores a half-applied peer change.
func (v *VPN) RecoverTransaction() error {
	content, err := os.ReadFile(v.transactionFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pending transaction
	if err := json.Unmarshal(content, &pending); err != nil {
		return err
	}
	if err := writePrivate(v.config.ConfigPath, []byte(pending.Config)); err != nil {
		return err
	}
	if err := writePrivate(v.stateFile(), []byte(pending.State)); err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(v.config.Profiles, pending.ID+".conf"), []byte(pending.Profile)); err != nil {
		return err
	}
	if err := v.ApplyRuntime(); err != nil {
		return err
	}
	return os.Remove(v.transactionFile())
}

// Inventory returns the current peer state with live handshake counters.
func (v *VPN) Inventory() ([]PeerInfo, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	state, err := v.LoadState()
	if err != nil {
		return nil, err
	}
	configContent, err := os.ReadFile(v.config.ConfigPath)
	if err != nil {
		return nil, err
	}
	current := map[string]bool{}
	for _, section := range splitPeers(string(configContent)) {
		current[sectionValues(section)["PublicKey"]] = true
	}
	dump, err := v.run([]string{"awg", "show", "awg0", "dump"}, "")
	if err != nil {
		return nil, err
	}
	stats := map[string][]string{}
	for index, line := range strings.Split(dump, "\n") {
		if index == 0 {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) >= 8 {
			stats[fields[0]] = fields
		}
	}
	result := make([]PeerInfo, 0, len(state))
	for peerID, record := range state {
		stat := stats[record.Public]
		info := PeerInfo{
			ID: peerID, Name: v.names[peerID], Address: v.addrs[peerID],
			Enabled: current[record.Public], Protected: peerID == "nikita",
		}
		if stat != nil {
			info.LatestHandshake = parseInt(stat[4])
			info.ReceivedBytes = parseInt(stat[5])
			info.SentBytes = parseInt(stat[6])
		}
		result = append(result, info)
	}
	return result, nil
}

func parseInt(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// Change enables, disables or rotates one unprotected peer transactionally.
func (v *VPN) Change(peerID, action string) (map[string]any, error) {
	if (peerID != "egor" && peerID != "leonid" && peerID != "igor") ||
		(action != "enable" && action != "disable" && action != "rotate") {
		return nil, errors.New("protected or invalid peer operation")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	state, err := v.LoadState()
	if err != nil {
		return nil, err
	}
	record := state[peerID]
	originalContent, err := os.ReadFile(v.config.ConfigPath)
	if err != nil {
		return nil, err
	}
	original := string(originalContent)
	profilePath := filepath.Join(v.config.Profiles, peerID+".conf")
	profileContent, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, err
	}
	oldProfile := string(profileContent)
	oldState, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	sections := strings.Split(original, "[Peer]")
	found := []string{}
	retained := []string{}
	for _, section := range sections[1:] {
		if sectionValues(section)["PublicKey"] == record.Public {
			found = append(found, section)
		} else {
			retained = append(retained, section)
		}
	}
	record.Enabled = len(found) > 0
	if len(found) > 0 {
		record.Peer = found[0]
	}
	profile := oldProfile
	var public string
	if action == "rotate" {
		private, err := v.run([]string{"awg", "genkey"}, "")
		if err != nil {
			return nil, err
		}
		public, err = v.run([]string{"awg", "pubkey"}, private)
		if err != nil {
			return nil, err
		}
		psk, err := v.run([]string{"awg", "genpsk"}, "")
		if err != nil {
			return nil, err
		}
		if profile, err = ReplaceField(profile, "PrivateKey", private); err != nil {
			return nil, err
		}
		if profile, err = ReplaceField(profile, "PresharedKey", psk); err != nil {
			return nil, err
		}
		if record.Peer, err = ReplaceField(record.Peer, "PublicKey", public); err != nil {
			return nil, err
		}
		if record.Peer, err = ReplaceField(record.Peer, "PresharedKey", psk); err != nil {
			return nil, err
		}
	}
	switch action {
	case "enable":
		record.Enabled = true
	case "disable":
		record.Enabled = false
	}
	if action == "rotate" {
		record.Public = public
	}
	if record.Enabled {
		retained = append(retained, record.Peer)
	}
	updated := sections[0]
	for _, section := range retained {
		updated += "[Peer]" + section
	}
	pending, err := json.Marshal(transaction{Config: original, State: string(oldState), Profile: oldProfile, ID: peerID})
	if err != nil {
		return nil, err
	}
	if err := writePrivate(v.transactionFile(), pending); err != nil {
		return nil, err
	}
	state[peerID] = record
	stateContent, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	apply := func() error {
		if err := writePrivate(v.config.ConfigPath, []byte(updated)); err != nil {
			return err
		}
		if err := v.ApplyRuntime(); err != nil {
			return err
		}
		if err := writePrivate(profilePath, []byte(profile)); err != nil {
			return err
		}
		return writePrivate(v.stateFile(), stateContent)
	}
	if err := apply(); err != nil {
		if recoverErr := v.recoverLocked(); recoverErr != nil {
			return nil, fmt.Errorf("%w; recovery failed: %v", err, recoverErr)
		}
		return nil, err
	}
	if err := os.Remove(v.transactionFile()); err != nil {
		return nil, err
	}
	result := map[string]any{"id": peerID}
	if action == "rotate" {
		result["configuration"] = profile
	}
	return result, nil
}

func (v *VPN) recoverLocked() error {
	content, err := os.ReadFile(v.transactionFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pending transaction
	if err := json.Unmarshal(content, &pending); err != nil {
		return err
	}
	if err := writePrivate(v.config.ConfigPath, []byte(pending.Config)); err != nil {
		return err
	}
	if err := writePrivate(v.stateFile(), []byte(pending.State)); err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(v.config.Profiles, pending.ID+".conf"), []byte(pending.Profile)); err != nil {
		return err
	}
	if err := v.ApplyRuntime(); err != nil {
		return err
	}
	return os.Remove(v.transactionFile())
}

// Lock serializes external operations with the agent handler.
func (v *VPN) Lock() { v.mu.Lock() }

// Unlock releases the agent lock (used by callers that already locked).
func (v *VPN) Unlock() { v.mu.Unlock() }
