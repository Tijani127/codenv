package config

import "testing"

// TestMergeKeepsBucketsFromBothSides guards a real defect: Merge copied the
// receiver's buckets but never appended the other side's, so an included
// config's buckets were dropped and never cloned. A project declaring the
// java bucket then resolved no manifests at all.
func TestMergeKeepsBucketsFromBothSides(t *testing.T) {
	base := NewConfig()
	base.Buckets = []string{"nirsoft"}
	base.Packages = Packages{"main/go"}

	other := NewConfig()
	other.Buckets = []string{"java"}
	other.Packages = Packages{"java/temurin21-jdk"}

	got := base.Merge(other)
	if len(got.Buckets) != 2 {
		t.Fatalf("buckets = %v, want both nirsoft and java", got.Buckets)
	}
	seen := map[string]bool{}
	for _, b := range got.Buckets {
		seen[b] = true
	}
	if !seen["java"] || !seen["nirsoft"] {
		t.Errorf("buckets = %v, want both", got.Buckets)
	}
	if len(got.Packages) != 2 {
		t.Errorf("packages = %v, want both", got.Packages)
	}
}

func TestMergeDeduplicatesBuckets(t *testing.T) {
	base := NewConfig()
	base.Buckets = []string{"java"}
	other := NewConfig()
	other.Buckets = []string{"java"}

	got := base.Merge(other)
	if len(got.Buckets) != 1 {
		t.Errorf("buckets = %v, want a single entry", got.Buckets)
	}
}

// TestMergeWithoutOtherBucketsIsUnchanged makes sure the fix did not make a
// merge invent buckets.
func TestMergeWithoutOtherBucketsIsUnchanged(t *testing.T) {
	base := NewConfig()
	base.Buckets = []string{"java"}
	got := base.Merge(NewConfig())
	if len(got.Buckets) != 1 || got.Buckets[0] != "java" {
		t.Errorf("buckets = %v, want [java]", got.Buckets)
	}
	if len(base.Buckets) != 1 || base.Buckets[0] != "java" {
		t.Errorf("Merge mutated the receiver: %v", base.Buckets)
	}
}
