package secrets

import (
	"testing"

	"github.com/Tijani127/codenv/internal/config"
)

func env(pairs map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := pairs[k]
		return v, ok
	}
}

func TestResolveReadsFromEnvironment(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{Names: []string{"API_KEY"}}}
	got, err := Resolve(cfg, env(map[string]string{"API_KEY": "s3cret"}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["API_KEY"] != "s3cret" {
		t.Errorf("API_KEY = %q, want s3cret", got.Values["API_KEY"])
	}
	if len(got.Missing) != 0 {
		t.Errorf("Missing = %v, want empty", got.Missing)
	}
}

func TestResolveMissingIsReportedNotSilentlyEmpty(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{Names: []string{"API_KEY"}}}
	got, err := Resolve(cfg, env(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Redacted["API_KEY"] {
		t.Error("API_KEY should be marked redacted when absent")
	}
	if _, present := got.Values["API_KEY"]; present {
		t.Error("Values must not contain a key with no value")
	}
}

func TestResolveEmptyStringCountsAsMissing(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{Names: []string{"API_KEY"}}}
	got, _ := Resolve(cfg, env(map[string]string{"API_KEY": ""}))
	if _, present := got.Values["API_KEY"]; present {
		t.Error("an empty env var must be treated as missing, not as an empty secret")
	}
}

func TestResolveFromMapping(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{
		Names: []string{"DATABASE_URL"},
		From:  map[string]string{"DATABASE_URL": "PG_URL"},
	}}
	got, _ := Resolve(cfg, env(map[string]string{"PG_URL": "postgres://x"}))
	if got.Values["DATABASE_URL"] != "postgres://x" {
		t.Errorf("DATABASE_URL = %q", got.Values["DATABASE_URL"])
	}
}

func TestResolveMultipleSourcesFirstWins(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{
		Names: []string{"API_KEY"},
		From:  map[string]string{"API_KEY": "PRIMARY,SECONDARY"},
	}}
	got, _ := Resolve(cfg, env(map[string]string{"PRIMARY": "one", "SECONDARY": "two"}))
	if got.Values["API_KEY"] != "one" {
		t.Errorf("API_KEY = %q, want the first available source", got.Values["API_KEY"])
	}
}

func TestResolveFallsBackToNextSource(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{
		Names: []string{"API_KEY"},
		From:  map[string]string{"API_KEY": "MISSING,PRESENT"},
	}}
	got, _ := Resolve(cfg, env(map[string]string{"PRESENT": "two"}))
	if got.Values["API_KEY"] != "two" {
		t.Errorf("API_KEY = %q, want fallback to the second source", got.Values["API_KEY"])
	}
}

func TestReportFailsWhenSourceMissing(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{
		Names: []string{"API_KEY"},
		From:  map[string]string{"API_KEY": "PG_URL"},
	}}
	got, _ := Resolve(cfg, env(map[string]string{}))
	if err := got.Report(); err == nil {
		t.Error("Report must fail when a required source variable is unset")
	}
}

func TestReportPassesWhenAllPresent(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{
		Names: []string{"API_KEY"},
		From:  map[string]string{"API_KEY": "PG_URL"},
	}}
	got, _ := Resolve(cfg, env(map[string]string{"PG_URL": "v"}))
	if err := got.Report(); err != nil {
		t.Errorf("Report should pass, got %v", err)
	}
}

func TestConfigIsSecret(t *testing.T) {
	cfg := &config.Config{Secrets: &config.Secrets{Names: []string{"A", "B"}}}
	if !cfg.IsSecret("A") || !cfg.IsSecret("B") {
		t.Error("IsSecret should find declared names")
	}
	if cfg.IsSecret("C") {
		t.Error("IsSecret should not match undeclared names")
	}
}
