package pkgspec

import (
	"fmt"
	"sort"
	"strings"
)

type Spec struct {
	Raw     string
	Bucket  string
	Name    string
	Version string
}

func Parse(raw string) (Spec, error) {
	s := Spec{Raw: strings.TrimSpace(raw)}
	if s.Raw == "" {
		return s, fmt.Errorf("empty package reference")
	}
	if idx := strings.LastIndex(s.Raw, "@"); idx > 0 {
		s.Version = strings.TrimSpace(s.Raw[idx+1:])
		s.Raw = strings.TrimSpace(s.Raw[:idx])
		if s.Version == "" {
			return s, fmt.Errorf("package %q has an empty version", raw)
		}
		if !validVersionChars(s.Version) {
			return s, fmt.Errorf("package %q has an invalid version %q (allowed: letters, digits, dot, dash, plus)", raw, s.Version)
		}
	}
	if strings.HasPrefix(s.Raw, "bucket/") {
		return s, fmt.Errorf("package %q is missing a name after the bucket", raw)
	}
	if idx := strings.Index(s.Raw, "/"); idx >= 0 {
		s.Bucket = strings.ToLower(strings.TrimSpace(s.Raw[:idx]))
		s.Name = strings.ToLower(strings.TrimSpace(s.Raw[idx+1:]))
	} else {
		s.Name = strings.ToLower(s.Raw)
	}
	if s.Name == "" {
		return s, fmt.Errorf("package %q is missing a name", raw)
	}
	if strings.ContainsAny(s.Name, "@/\\") {
		return s, fmt.Errorf("invalid package name %q", s.Name)
	}
	return s, nil
}

func validVersionChars(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '+', r == '_':
		default:
			return false
		}
	}
	return v != ""
}

func (s Spec) Qualified() string {
	if s.Bucket == "" {
		return s.Name
	}
	return s.Bucket + "/" + s.Name
}

func (s Spec) WithVersion(v string) Spec {
	s.Version = v
	return s
}

func (s Spec) Key(version string) string {
	if version == "" {
		version = s.Version
	}
	if version == "" {
		return s.Qualified()
	}
	return s.Qualified() + "@" + version
}

func (s Spec) String() string {
	if s.Version == "" {
		return s.Qualified()
	}
	return s.Qualified() + "@" + s.Version
}

func (s Spec) StringWith(version string) string {
	if version == "" {
		version = s.Version
	}
	if version == "" {
		return s.Qualified()
	}
	return s.Qualified() + "@" + version
}

func Normalize(list []string) ([]Spec, error) {
	var out []Spec
	seen := map[string]bool{}
	for _, raw := range list {
		s, err := Parse(raw)
		if err != nil {
			return nil, err
		}
		if seen[s.Qualified()] {
			continue
		}
		seen[s.Qualified()] = true
		out = append(out, s)
	}
	return out, nil
}

func NormalizeSpecs(list []Spec) []Spec {
	var out []Spec
	seen := map[string]bool{}
	for _, s := range list {
		if s.Qualified() == "" || seen[s.Qualified()] {
			continue
		}
		seen[s.Qualified()] = true
		out = append(out, s)
	}
	return out
}

func Remove(list []Spec, targets []Spec) []Spec {
	drop := map[string]bool{}
	for _, t := range targets {
		drop[t.Qualified()] = true
		if t.Bucket == "" {
			drop[t.Name] = true
		}
		drop[t.Name] = true
	}
	var out []Spec
	for _, s := range list {
		if drop[s.Qualified()] || drop[s.Name] {
			continue
		}
		out = append(out, s)
	}
	return out
}

func Sort(list []Spec) []Spec {
	out := append([]Spec(nil), list...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Bucket != out[j].Bucket {
			return out[i].Bucket < out[j].Bucket
		}
		return out[i].Name < out[j].Name
	})
	return out
}
