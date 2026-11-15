package ops

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProtectedPeerIsRejectedBeforeStateAccess(t *testing.T) {
	directory := t.TempDir()
	controller := NewVPN(VPNConfig{
		ConfigPath: filepath.Join(directory, "missing.conf"),
		Profiles:   directory,
		StatePath:  filepath.Join(directory, "state", "state.json"),
	})
	called := false
	controller.SetCommand(func([]string, string) (string, error) {
		called = true
		return "", errors.New("must not run")
	})
	for _, peer := range []string{"nikita", "server2", "../nikita", ""} {
		for _, action := range []string{"disable", "rotate", "enable"} {
			if _, err := controller.Change(peer, action); err == nil {
				t.Fatalf("%s/%s accepted", peer, action)
			}
		}
	}
	if called {
		t.Fatal("protected peer reached the command runner")
	}
}

func TestReplaceFieldRejectsDuplicateKeys(t *testing.T) {
	if _, err := ReplaceField("PrivateKey = a\nPrivateKey = b\n", "PrivateKey", "c"); err == nil {
		t.Fatal("duplicate field accepted")
	}
	updated, err := ReplaceField("[Interface]\nPrivateKey = a\n", "PrivateKey", "b")
	if err != nil || updated != "[Interface]\nPrivateKey = b\n" {
		t.Fatalf("updated = %q, %v", updated, err)
	}
}

func TestFailedRuntimeRestoresProfileAndState(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "awg0.conf")
	statePath := filepath.Join(directory, "state", "state.json")
	profilePath := filepath.Join(directory, "egor.conf")
	originalConfig := "[Interface]\nAddress = 10.77.0.1\n[Peer]\nPublicKey = old\nAllowedIPs = 10.77.0.3/32\n"
	originalProfile := "[Interface]\nPrivateKey = original\n"
	if err := os.WriteFile(configPath, []byte(originalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, []byte(originalProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	record := PeerRecord{Public: "old", Peer: "\nPublicKey = old\nAllowedIPs = 10.77.0.3/32\n", Enabled: true}
	stateContent, _ := json.Marshal(map[string]PeerRecord{"egor": record})
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stateContent, 0o600); err != nil {
		t.Fatal(err)
	}

	controller := NewVPN(VPNConfig{ConfigPath: configPath, Profiles: directory, StatePath: statePath})
	syncCalls := 0
	controller.SetCommand(func(args []string, input string) (string, error) {
		switch {
		case args[0] == "awg-quick" && len(args) > 1 && args[1] == "strip":
			return "stripped", nil
		case args[0] == "awg" && len(args) > 1 && args[1] == "syncconf":
			syncCalls++
			if syncCalls == 1 {
				return "", errors.New("sync failed")
			}
			return "", nil
		default:
			return "", errors.New("unexpected command")
		}
	})

	if _, err := controller.Change("egor", "disable"); err == nil {
		t.Fatal("failed runtime change reported success")
	}
	configContent, _ := os.ReadFile(configPath)
	profileContent, _ := os.ReadFile(profilePath)
	if string(configContent) != originalConfig {
		t.Fatalf("config = %q", configContent)
	}
	if string(profileContent) != originalProfile {
		t.Fatalf("profile = %q", profileContent)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(statePath), "transaction.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("transaction journal was not removed")
	}
	restored, _ := os.ReadFile(statePath)
	var records map[string]PeerRecord
	if err := json.Unmarshal(restored, &records); err != nil || records["egor"].Enabled != true {
		t.Fatalf("state was not restored: %s", restored)
	}
}
