package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type Index struct {
	path string
	Apps map[string]map[string][]string `json:"apps"`
}

func LoadIndex(s *Store) (*Index, error) {
	ix := &Index{path: s.IndexPath(), Apps: map[string]map[string][]string{}}
	data, err := os.ReadFile(s.IndexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return ix, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return ix, nil
	}
	if err := json.Unmarshal(data, ix); err != nil {
		return &Index{path: s.IndexPath(), Apps: map[string]map[string][]string{}}, nil
	}
	if ix.Apps == nil {
		ix.Apps = map[string]map[string][]string{}
	}
	return ix, nil
}

func (ix *Index) Dirs(app, version string) []string {
	byVersion, ok := ix.Apps[app]
	if !ok {
		return nil
	}
	return byVersion[version]
}

func (ix *Index) KnownVersions(app string) []string {
	byVersion := ix.Apps[app]
	out := make([]string, 0, len(byVersion))
	for v := range byVersion {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (ix *Index) Has(app, version string) bool {
	return len(ix.Dirs(app, version)) > 0
}

func (ix *Index) Set(app, version string, dirs []string) {
	if version == "" {
		return
	}
	if ix.Apps == nil {
		ix.Apps = map[string]map[string][]string{}
	}
	byVersion, ok := ix.Apps[app]
	if !ok {
		byVersion = map[string][]string{}
		ix.Apps[app] = byVersion
	}
	byVersion[version] = dedupe(dirs)
}

func (ix *Index) Remove(app, version string) {
	byVersion, ok := ix.Apps[app]
	if !ok {
		return
	}
	delete(byVersion, version)
	if len(byVersion) == 0 {
		delete(ix.Apps, app)
	}
}

func (ix *Index) Merge(other *Index) {
	for app, byVersion := range other.Apps {
		for version, dirs := range byVersion {
			existing := ix.Apps[app]
			if existing == nil {
				existing = map[string][]string{}
				ix.Apps[app] = existing
			}
			existing[version] = dedupe(append(existing[version], dirs...))
		}
	}
}

func (ix *Index) Save() error {
	if ix.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ix.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ix, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ix.path, append(data, '\n'), 0o644)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if v == "" {
			continue
		}
		key := filepath.Clean(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
