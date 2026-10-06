package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	FileName   = "codenv.json"
	LockName   = "codenv.lock.json"
	LockSchema = 1
)

type Shell struct {
	InitHook string            `json:"init_hook,omitempty"`
	Scripts  map[string]string `json:"scripts,omitempty"`
}

type Packages []string

func (p *Packages) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case trimmed == "" || trimmed == "null" || trimmed == `""`:
		*p = nil
		return nil
	case strings.HasPrefix(trimmed, "["):
		var list []string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*p = list
		return nil
	case strings.HasPrefix(trimmed, "{"):
		var mapping map[string]any
		if err := json.Unmarshal(data, &mapping); err != nil {
			return err
		}
		list := make([]string, 0, len(mapping))
		for name, v := range mapping {
			switch t := v.(type) {
			case nil:
				list = append(list, name)
			case string:
				if t == "" || t == "latest" {
					list = append(list, name)
				} else {
					list = append(list, name+"@"+t)
				}
			case bool:
				if t {
					list = append(list, name)
				}
			case map[string]any:
				if ver, ok := t["version"].(string); ok && ver != "" {
					list = append(list, name+"@"+ver)
				} else {
					list = append(list, name)
				}
			default:
				return fmt.Errorf("invalid package entry for %q", name)
			}
		}
		sort.Strings(list)
		*p = list
		return nil
	}
	return fmt.Errorf("packages must be a list or an object")
}

type Config struct {
	Packages Packages          `json:"packages,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Shell    *Shell            `json:"shell,omitempty"`
	Include  []string          `json:"include,omitempty"`
	Buckets  []string          `json:"buckets,omitempty"`
	Store    string            `json:"store,omitempty"`
	Secrets  *Secrets          `json:"secrets,omitempty"`
}

type Secrets struct {
	Names []string          `json:"names,omitempty"`
	From  map[string]string `json:"from,omitempty"`
}

func (s *Secrets) Empty() bool {
	return s == nil || (len(s.Names) == 0 && len(s.From) == 0)
}

func (c *Config) SecretNames() []string {
	if c.Secrets == nil {
		return nil
	}
	return c.Secrets.Names
}

func (c *Config) SecretSources() []string {
	if c.Secrets == nil || c.Secrets.From == nil {
		return nil
	}
	names := make([]string, 0, len(c.Secrets.From))
	for k := range c.Secrets.From {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (c *Config) SourceFor(name string) string {
	if c.Secrets == nil || c.Secrets.From == nil {
		return ""
	}
	return c.Secrets.From[name]
}

func (c *Config) HasSecretMapping(name string) bool {
	if c.Secrets == nil || c.Secrets.From == nil {
		return false
	}
	_, ok := c.Secrets.From[name]
	return ok
}

func (c *Config) IsSecret(name string) bool {
	if c.Secrets == nil {
		return false
	}
	for _, n := range c.Secrets.Names {
		if n == name {
			return true
		}
	}
	return false
}

func (c *Config) EnsureSecrets() *Secrets {
	if c.Secrets == nil {
		c.Secrets = &Secrets{}
	}
	if c.Secrets.From == nil {
		c.Secrets.From = map[string]string{}
	}
	return c.Secrets
}

type LockedPackage struct {
	Bucket       string   `json:"bucket"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Architecture string   `json:"architecture"`
	Hash         string   `json:"hash,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Unresolved   bool     `json:"unresolved,omitempty"`
}

type Lock struct {
	LockVersion int                      `json:"lockVersion"`
	Generated   string                   `json:"generated,omitempty"`
	Top         []string                 `json:"top"`
	Resolved    map[string]LockedPackage `json:"resolved"`
}

func (p LockedPackage) Qualified() string {
	if p.Bucket == "" {
		return p.Name
	}
	return p.Bucket + "/" + p.Name
}

func (p LockedPackage) String() string {
	return p.StringWithVersion(p.Version)
}

func (p LockedPackage) StringWithVersion(v string) string {
	if v == "" {
		return p.Qualified()
	}
	return p.Qualified() + "@" + v
}

func NewLock() *Lock {
	return &Lock{
		LockVersion: LockSchema,
		Top:         []string{},
		Resolved:    map[string]LockedPackage{},
	}
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) InitHook() string {
	if c.Shell == nil {
		return ""
	}
	return c.Shell.InitHook
}

func (c *Config) Script(name string) (string, bool) {
	if c.Shell == nil || c.Shell.Scripts == nil {
		return "", false
	}
	v, ok := c.Shell.Scripts[name]
	return v, ok
}

func (c *Config) ScriptNames() []string {
	if c.Shell == nil {
		return nil
	}
	names := make([]string, 0, len(c.Shell.Scripts))
	for k := range c.Shell.Scripts {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (c *Config) EnsureShell() *Shell {
	if c.Shell == nil {
		c.Shell = &Shell{}
	}
	if c.Shell.Scripts == nil {
		c.Shell.Scripts = map[string]string{}
	}
	return c.Shell
}

func (c *Config) StoreMode() string {
	if strings.EqualFold(c.Store, "project") {
		return "project"
	}
	return "shared"
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out := *c
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func LoadLock(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewLock(), nil
		}
		return nil, err
	}
	l := NewLock()
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if l.Resolved == nil {
		l.Resolved = map[string]LockedPackage{}
	}
	if l.Top == nil {
		l.Top = []string{}
	}
	return l, nil
}

func SaveLock(path string, l *Lock) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	l.LockVersion = LockSchema
	if l.Generated == "" {
		l.Generated = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (c *Config) Merge(other *Config) *Config {
	out := &Config{
		Packages: append(Packages{}, c.Packages...),
		Env:      map[string]string{},
		Include:  append([]string{}, c.Include...),
		Buckets:  append([]string{}, c.Buckets...),
		Store:    c.Store,
	}
	for k, v := range c.Env {
		out.Env[k] = v
	}
	for k, v := range other.Env {
		out.Env[k] = v
	}
	if c.Shell != nil || other.Shell != nil {
		merged := &Shell{}
		if c.Shell != nil {
			merged.InitHook = c.Shell.InitHook
		}
		if other.Shell != nil && other.Shell.InitHook != "" {
			merged.InitHook = other.Shell.InitHook
		}
		if (c.Shell != nil && len(c.Shell.Scripts) > 0) || (other.Shell != nil && len(other.Shell.Scripts) > 0) {
			merged.Scripts = map[string]string{}
			if c.Shell != nil {
				for k, v := range c.Shell.Scripts {
					merged.Scripts[k] = v
				}
			}
			if other.Shell != nil {
				for k, v := range other.Shell.Scripts {
					merged.Scripts[k] = v
				}
			}
		}
		out.Shell = merged
	}
	if other.Store != "" {
		out.Store = other.Store
	}
	out.Secrets = mergeSecrets(c.Secrets, other.Secrets)
	out.Packages = append(out.Packages, other.Packages...)
	return out
}

func mergeSecrets(base, override *Secrets) *Secrets {
	if base == nil && override == nil {
		return nil
	}
	out := &Secrets{}
	var names []string
	if base != nil {
		names = append(names, base.Names...)
	}
	if override != nil {
		names = append(names, override.Names...)
	}
	out.Names = dedupeStrings(names)
	from := map[string]string{}
	if base != nil {
		for k, v := range base.From {
			from[k] = v
		}
	}
	if override != nil {
		for k, v := range override.From {
			from[k] = v
		}
	}
	if len(from) > 0 {
		out.From = from
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
