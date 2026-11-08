// Package gc reclaims store space by deleting package versions that no
// project still pins. It is deliberately pure: planning a sweep only needs
// the store layout and the index, so it is testable without Scoop.
package gc

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/store"
)

// Ref is a (package, version) pair that must survive a sweep.
type Ref struct {
	App     string
	Version string
}

// Entry is one version directory that a sweep wants to delete, or one index
// entry whose version directory is already gone (Stale).
type Entry struct {
	App     string
	Version string
	Path    string
	Bytes   int64
}

// Sweep is the plan for one store. Entries are the versions that can be
// deleted; Stale are index entries for versions that no longer exist on disk
// and just need pruning.
type Sweep struct {
	Entries []Entry
	Stale   []Entry
	Bytes   int64
}

// guardApps always survive a sweep. The shared store keeps codenv's own Scoop
// installation under apps/scoop/current; deleting it would force a full
// re-bootstrap for nothing.
var guardApps = map[string]bool{"scoop": true}

// RefsOf collects the pinned (package, version) pairs from a lockfile.
func RefsOf(lock *config.Lock) []Ref {
	var refs []Ref
	if lock == nil {
		return nil
	}
	for _, pkg := range lock.Resolved {
		if pkg.Name == "" || pkg.Version == "" {
			continue
		}
		refs = append(refs, Ref{App: pkg.Name, Version: pkg.Version})
	}
	return refs
}

// referenced reports whether a (package, version) pair is pinned by some
// project that codenv knows about.
func referenced(pinned map[Ref]bool, app, version string) bool {
	return pinned[Ref{App: app, Version: version}] ||
		pinned[Ref{App: app, Version: filepath.Clean(version)}]
}

// SweepStore plans a sweep of st: every installed version directory that no
// ref pins and that is not the target of the app's current junction. Index
// entries for versions that no longer exist on disk are reported as stale so
// the caller can prune them.
func SweepStore(st *store.Store, ix *store.Index, refs []Ref) *Sweep {
	pinned := make(map[Ref]bool, len(refs))
	for _, r := range refs {
		pinned[r] = true
	}
	out := &Sweep{}

	for _, app := range st.InstalledApps() {
		if guardApps[app] {
			continue
		}
		current, _ := st.ResolveCurrentVersion(app)
		for _, v := range st.InstalledVersions(app) {
			if referenced(pinned, app, v) || v == current {
				continue
			}
			vdir := st.VersionDir(app, v)
			size, err := dirSize(vdir)
			if err != nil {
				size = 0
			}
			out.Entries = append(out.Entries, Entry{App: app, Version: v, Path: vdir, Bytes: size})
			out.Bytes += size
		}
	}

	for app, byVersion := range ix.Apps {
		if guardApps[app] {
			continue
		}
		for v := range byVersion {
			if !st.HasVersion(app, v) {
				out.Stale = append(out.Stale, Entry{App: app, Version: v, Path: st.VersionDir(app, v)})
			}
		}
	}

	sortEntries(out.Entries)
	sortEntries(out.Stale)
	return out
}

// Apply deletes the planned version directories and prunes the index. Saving
// the index afterwards is the caller's job, so a dry run can Skip Apply and
// leave everything untouched.
func (s *Sweep) Apply(st *store.Store, ix *store.Index) error {
	var errs []string
	for _, e := range s.Entries {
		if err := os.RemoveAll(e.Path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", e.Path, err))
			continue
		}
		ix.Remove(e.App, e.Version)
		// Once the last version of an app is gone, the app directory holds
		// nothing but a dangling current junction; remove it too.
		if remaining := st.InstalledVersions(e.App); len(remaining) == 0 {
			_ = os.RemoveAll(st.AppDir(e.App))
		}
	}
	for _, e := range s.Stale {
		ix.Remove(e.App, e.Version)
	}
	if len(errs) > 0 {
		return fmt.Errorf("deleting store versions: %s", strings.Join(errs, "; "))
	}
	return nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].App != entries[j].App {
			return entries[i].App < entries[j].App
		}
		return entries[i].Version < entries[j].Version
	})
}

// dirSize sums the file sizes under root, ignoring unreadable entries.
// Junctions count as zero-size files rather than being followed, keeping the
// walk inside the version directory.
func dirSize(root string) (int64, error) {
	var out int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				out += info.Size()
			}
		}
		return nil
	})
	return out, err
}
