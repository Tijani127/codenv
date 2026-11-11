package cmd

import (
	"reflect"
	"testing"

	"github.com/Tijani127/codenv/internal/templates"
)

func TestTemplateScriptsReadsBuiltinCodenvJSON(t *testing.T) {
	files, err := templates.Files("go")
	if err != nil {
		t.Fatalf("loading builtin go: %s", err)
	}
	scripts := templateScripts(files)
	want := []string{"build", "check", "fmt", "run", "test", "vet"}
	if !reflect.DeepEqual(scripts, want) {
		t.Errorf("templateScripts(go) = %v, want %v", scripts, want)
	}
}

func TestTemplateScriptsHandlesMissingAndMalformed(t *testing.T) {
	if got := templateScripts(nil); got != nil {
		t.Errorf("templateScripts(nil) = %v, want nil", got)
	}
	if got := templateScripts(map[string][]byte{"codenv.json": []byte("not json")}); got != nil {
		t.Errorf("templateScripts(malformed) = %v, want nil", got)
	}
	if got := templateScripts(map[string][]byte{"codenv.json": []byte(`{"shell":{}}`)}); got != nil {
		t.Errorf("templateScripts(no scripts) = %v, want nil", got)
	}
}
