package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Tijani127/codenv/internal/store"
)

// The registry records every project directory that has used codenv, with the
// store mode it runs in. 'codenv gc' reads it to decide which store versions
// are still pinned by some project's lockfile — without it, gc could delete a
// version that a project on disk still pins but has not opened recently.

const registryName = "registry.json"

type RegistryEntry struct {
	Dir  string `json:"dir"`
	Mode string `json:"mode"`
	Last string `json:"last"`
}

type Registry struct {
	Projects []RegistryEntry `json:"projects"`
}

// RegistryPath returns the registry file inside the codenv home.
func RegistryPath() (string, error) {
	home, err := store.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, registryName), nil
}

// LoadRegistry reads the registry; a missing file is an empty registry.
func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return &Registry{}, nil
	}
	return &reg, nil
}

// Touch registers (or refreshes the timestamp of) a project in the registry.
func Touch(dir, mode string) {
	path, err := RegistryPath()
	if err != nil {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	abs = filepath.Clean(abs)

	reg, err := LoadRegistry()
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i, e := range reg.Projects {
		if e.Dir == abs {
			reg.Projects[i].Mode = mode
			reg.Projects[i].Last = now
			_ = reg.save(path)
			return
		}
	}
	reg.Projects = append(reg.Projects, RegistryEntry{Dir: abs, Mode: mode, Last: now})
	_ = reg.save(path)
}

func (r *Registry) save(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
