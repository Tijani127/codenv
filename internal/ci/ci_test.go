package ci

import (
	"strings"
	"testing"
)

func TestRenderLatest(t *testing.T) {
	out, err := Render(RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	checks := []string{
		"name: codenv",
		"runs-on: windows-latest",
		"codenv-buckets-v1",
		"codenv-store-${{ runner.arch }}-${{ hashFiles('codenv.lock.json') }}",
		"releases/latest/download/codenv-windows-",
		"run: codenv run test",
	}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Fatalf("rendered workflow is missing %q:\n%s", c, s)
		}
	}
}

func TestRenderPinnedRefAndCommand(t *testing.T) {
	out, err := Render(RenderOptions{CodenvRef: "v1.1.0", Command: []string{"run", "ci"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "releases/download/v1.1.0/") {
		t.Fatalf("expected pinned release URL, got:\n%s", s)
	}
	if !strings.Contains(s, "run: codenv run ci") {
		t.Fatalf("expected command override, got:\n%s", s)
	}
	if strings.Contains(s, "latest/download") {
		t.Fatalf("expected no latest URL for a pinned ref:\n%s", s)
	}
}
