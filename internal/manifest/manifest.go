package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/store"
)

type Arch struct {
	URL          string `json:"url"`
	Dependencies any    `json:"dependencies"`
	Bin          any    `json:"bin"`
}

type Manifest struct {
	Path         string          `json:"-"`
	Bucket       string          `json:"-"`
	Version      string          `json:"version"`
	Description  string          `json:"description"`
	Homepage     string          `json:"homepage"`
	License      any             `json:"license"`
	Notes        any             `json:"notes"`
	Dependencies any             `json:"dependencies"`
	Architecture map[string]Arch `json:"architecture"`
	Bin          any             `json:"bin"`
	Shortcuts    any             `json:"shortcuts"`
	CheckSHA256  string          `json:"checkver_scoop_checkSHA256"`
	PreInstall   any             `json:"pre_install"`
	PostInstall  any             `json:"post_install"`
	arch         string
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{t}
	case []string:
		return t
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

type Catalog struct {
	store *store.Store
	arch  string
}

func New(s *store.Store, arch string) *Catalog {
	return &Catalog{store: s, arch: arch}
}

func (c *Catalog) BucketOrder() []string {
	preferred := []string{"main", "extras", "versions"}
	seen := map[string]bool{}
	var out []string
	for _, p := range preferred {
		if c.store.HasBucket(p) {
			out = append(out, p)
			seen[p] = true
		}
	}
	var rest []string
	for _, b := range c.store.Buckets() {
		if !seen[b] {
			rest = append(rest, b)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func (c *Catalog) bucketManifestPath(bucket, name string) string {
	return filepath.Join(c.store.BucketsDir(), bucket, "bucket", name+".json")
}

func (c *Catalog) bucketVersionPath(bucket, name, version string) string {
	return filepath.Join(c.store.BucketsDir(), bucket, "bucket", name, version+".json")
}

func (c *Catalog) load(path, bucket string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m.Path = path
	m.Bucket = bucket
	m.arch = c.arch
	if m.Version == "" && m.Architecture != nil {
		if a, ok := m.Architecture[c.arch]; ok && a.URL != "" {
			m.Version = "arch-specific"
		}
	}
	return &m, nil
}

func (c *Catalog) Find(spec pkgspec.Spec) (*Manifest, error) {
	buckets := c.BucketOrder()
	if spec.Bucket != "" {
		buckets = []string{spec.Bucket}
	}
	if spec.Version != "" {
		for _, b := range buckets {
			p := c.bucketVersionPath(b, spec.Name, spec.Version)
			if _, err := os.Stat(p); err == nil {
				return c.load(p, b)
			}
		}
	}
	var firstErr error
	for _, b := range buckets {
		p := c.bucketManifestPath(b, spec.Name)
		m, err := c.load(p, b)
		if err != nil {
			if firstErr == nil && !os.IsNotExist(err) {
				firstErr = err
			}
			continue
		}
		if spec.Version != "" && m.Version != spec.Version {
			continue
		}
		return m, nil
	}
	if spec.Version != "" {
		return nil, fmt.Errorf("no manifest for %s", spec.String())
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("no manifest for %s", spec.Qualified())
}

func (c *Catalog) Latest(bucket, name string) (*Manifest, error) {
	spec := pkgspec.Spec{Bucket: bucket, Name: name}
	if bucket == "" {
		spec.Bucket = ""
	}
	return c.Find(spec)
}

func (c *Catalog) VersionDirs(bucket, name string) []string {
	base := filepath.Join(c.store.BucketsDir(), bucket, "bucket", name)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func versionMatches(have, want string) bool {
	if have == "" || have == "?" {
		return false
	}
	h := strings.ToLower(have)
	w := strings.ToLower(want)
	return h == w || strings.HasPrefix(h, w+".")
}

func (c *Catalog) Suggest(name, version string) []string {
	if version == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, bucket := range c.BucketOrder() {
		base := filepath.Join(c.store.BucketsDir(), bucket, "bucket")
		matches, _ := filepath.Glob(filepath.Join(base, "*.json"))
		sort.Strings(matches)
		for _, p := range matches {
			pkg := strings.TrimSuffix(filepath.Base(p), ".json")
			if !strings.Contains(strings.ToLower(pkg), strings.ToLower(name)) {
				continue
			}
			ver := ""
			if data, err := os.ReadFile(p); err == nil {
				var m Manifest
				if err := json.Unmarshal(data, &m); err == nil {
					ver = m.Version
				}
			}
			if !versionMatches(ver, version) {
				continue
			}
			ref := bucket + "/" + pkg
			if seen[ref] {
				continue
			}
			seen[ref] = true
			if ver != "" {
				ref += "@" + ver
			}
			out = append(out, ref)
		}
	}
	return out
}

func (c *Catalog) Dependencies(m *Manifest) []string {
	if m.Architecture != nil {
		if a, ok := m.Architecture[c.arch]; ok && a.Dependencies != nil {
			return normalizeList(toStringSlice(a.Dependencies))
		}
	}
	return normalizeList(toStringSlice(m.Dependencies))
}

func normalizeList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

func (m *Manifest) BinEntries() []string {
	if m.Architecture != nil {
		if a, ok := m.Architecture[m.arch]; ok && a.Bin != nil {
			return collectEntries(nil, a.Bin)
		}
	}
	return collectEntries(nil, m.Bin)
}

func collectEntries(out []string, v any) []string {
	appendEntry := func(e any) {
		switch t := e.(type) {
		case string:
			if t != "" {
				out = append(out, t)
			}
		case []any:
			if len(t) > 0 {
				if s, ok := t[0].(string); ok && s != "" {
					out = append(out, s)
				}
			}
		case map[string]any:
			for k := range t {
				out = append(out, k)
			}
		}
	}
	for _, e := range toEntries(v) {
		appendEntry(e)
	}
	return out
}

func toEntries(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	case []string:
		out := make([]any, 0, len(t))
		for _, s := range t {
			out = append(out, s)
		}
		return out
	case map[string]any:
		out := make([]any, 0, len(t))
		for _, val := range t {
			out = append(out, val)
		}
		return out
	default:
		return []any{v}
	}
}

func (m *Manifest) NoteLines() []string {
	return toStringSlice(m.Notes)
}

type Result struct {
	Bucket      string
	Name        string
	Version     string
	Description string
}

func (c *Catalog) Search(query string) ([]Result, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Result
	for _, bucket := range c.BucketOrder() {
		base := filepath.Join(c.store.BucketsDir(), bucket, "bucket")
		matches, err := filepath.Glob(filepath.Join(base, "*", "*.json"))
		if err != nil {
			continue
		}
		top, _ := filepath.Glob(filepath.Join(base, "*.json"))
		matches = append(matches, top...)
		for _, p := range matches {
			name := strings.TrimSuffix(filepath.Base(p), ".json")
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var m Manifest
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			hay := strings.ToLower(name + " " + m.Description)
			if q != "" && !strings.Contains(hay, q) {
				continue
			}
			out = append(out, Result{
				Bucket:      bucket,
				Name:        name,
				Version:     m.Version,
				Description: firstLine(m.Description),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Bucket < out[j].Bucket
	})
	return dedupeResults(out), nil
}

func dedupeResults(in []Result) []Result {
	seen := map[string]bool{}
	var out []Result
	for _, r := range in {
		k := r.Name + "/" + r.Bucket
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
