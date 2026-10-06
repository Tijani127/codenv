package templates

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListIsNonEmpty(t *testing.T) {
	list := List()
	if len(list) == 0 {
		t.Fatal("no templates defined")
	}
	seen := map[string]bool{}
	for _, tpl := range list {
		if tpl.Name == "" || tpl.Title == "" || tpl.Description == "" {
			t.Errorf("incomplete template: %+v", tpl)
		}
		if seen[tpl.Name] {
			t.Errorf("duplicate template name %q", tpl.Name)
		}
		seen[tpl.Name] = true
	}
}

func TestEveryListedTemplateHasFiles(t *testing.T) {
	for _, tpl := range List() {
		files, err := Files(tpl.Name)
		if err != nil {
			t.Errorf("Files(%q): %v", tpl.Name, err)
			continue
		}
		if len(files) == 0 {
			t.Errorf("template %q has no files", tpl.Name)
		}
		if _, ok := files["codenv.json"]; !ok {
			t.Errorf("template %q has no codenv.json", tpl.Name)
		}
	}
}

func TestCodenvJSONIsValidAfterSubstitution(t *testing.T) {
	for _, tpl := range List() {
		files, err := Files(tpl.Name)
		if err != nil {
			t.Fatalf("Files(%q): %v", tpl.Name, err)
		}
		raw := Substitute(string(files["codenv.json"]), "myproj", tpl.Title)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			t.Errorf("template %q produces invalid JSON: %v\n%s", tpl.Name, err, raw)
		}
		if _, ok := parsed["packages"]; !ok {
			t.Errorf("template %q codenv.json has no packages", tpl.Name)
		}
	}
}

func TestSubstituteReplacesAllPlaceholders(t *testing.T) {
	in := "{{PROJECT}} {{TITLE}} {{MODULE}}"
	got := Substitute(in, "myproj", "My Title")
	if strings.Contains(got, "{{") {
		t.Errorf("unsubstituted placeholder remains: %q", got)
	}
	if got != "myproj My Title github.com/Tijani127/myproj" {
		t.Errorf("Substitute = %q", got)
	}
}

func TestNoPlaceholdersRemainInAnyFile(t *testing.T) {
	for _, tpl := range List() {
		files, err := Files(tpl.Name)
		if err != nil {
			t.Fatalf("Files(%q): %v", tpl.Name, err)
		}
		for name, data := range files {
			out := Substitute(string(data), "myproj", tpl.Title)
			if strings.Contains(out, "{{") {
				t.Errorf("%s/%s has unsubstituted placeholders", tpl.Name, name)
			}
		}
	}
}

func TestExists(t *testing.T) {
	if !Exists("go") {
		t.Error("Exists(go) should be true")
	}
	if Exists("cobol") {
		t.Error("Exists(cobol) should be false")
	}
}

func TestGoTemplateHasBuildableFiles(t *testing.T) {
	files, err := Files("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"gomod.tmpl", "main.go.tmpl", "main_test.go.tmpl"} {
		if _, ok := files[want]; !ok {
			t.Errorf("go template missing %s", want)
		}
	}
	mod := Substitute(string(files["gomod.tmpl"]), "myproj", "Go")
	if !strings.Contains(mod, "module github.com/Tijani127/myproj") {
		t.Errorf("go.mod module line looks wrong: %q", mod)
	}
}
