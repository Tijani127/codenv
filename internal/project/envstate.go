package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The environment snapshot records the pins a shell last activated with.
// The next shell start compares against it, so a change that happened in
// between — a re-resolved pin, an 'codenv update', an edit to codenv.json —
// is announced in one line instead of being noticed only from a tool's
// version string mid-session.

const envStateName = "env.json"

type EnvSnapshot struct {
	// Pins is the sorted list of top-level "pkg@version" pins.
	Pins  []string `json:"pins"`
	Saved string   `json:"saved"`
}

func envStatePath(stateDir string) string {
	return filepath.Join(stateDir, envStateName)
}

// LoadEnvSnapshot reads the last shell's snapshot, or nil the first time.
func LoadEnvSnapshot(stateDir string) *EnvSnapshot {
	data, err := os.ReadFile(envStatePath(stateDir))
	if err != nil {
		return nil
	}
	var snap EnvSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil
	}
	return &snap
}

// SaveEnvSnapshot records the pins activated by this shell.
func SaveEnvSnapshot(stateDir string, pins []string) {
	snap := EnvSnapshot{Pins: pins, Saved: time.Now().UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(stateDir, 0o755)
	_ = os.WriteFile(envStatePath(stateDir), append(data, '\n'), 0o644)
}
