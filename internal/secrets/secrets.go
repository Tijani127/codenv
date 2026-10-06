package secrets

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
)

type Resolved struct {
	Values   map[string]string
	Missing  []string
	Redacted map[string]bool
}

func Resolve(cfg *config.Config, lookup func(string) (string, bool)) (*Resolved, error) {
	out := &Resolved{
		Values:   map[string]string{},
		Redacted: map[string]bool{},
	}
	if cfg == nil {
		return out, nil
	}

	for _, name := range cfg.SecretSources() {
		source, _ := cfg.Secrets.From[name]
		for _, varName := range splitNames(source) {
			if varName == "" {
				continue
			}
			value, ok := lookup(varName)
			if !ok || value == "" {
				out.Missing = append(out.Missing, varName)
				continue
			}
			if _, already := out.Values[name]; !already {
				out.Values[name] = value
			}
		}
	}

	for _, name := range cfg.SecretNames() {
		if _, ok := out.Values[name]; ok {
			continue
		}
		if value, ok := lookup(name); ok && value != "" {
			out.Values[name] = value
			continue
		}
		out.Redacted[name] = true
	}

	sort.Strings(out.Missing)
	out.Missing = dedupe(out.Missing)
	return out, nil
}

func ResolveFromEnv(cfg *config.Config) (*Resolved, error) {
	return Resolve(cfg, func(k string) (string, bool) {
		v, ok := os.LookupEnv(k)
		return v, ok
	})
}

func splitNames(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	return fields
}

func dedupe(in []string) []string {
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

func (r *Resolved) Apply(target map[string]string) {
	for k, v := range r.Values {
		target[k] = v
	}
}

func (r *Resolved) Report() error {
	if len(r.Missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"missing secret source variables: %s\n"+
			"set them in your environment before starting a codenv shell",
		strings.Join(r.Missing, ", "))
}
