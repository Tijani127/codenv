package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/manifest"
	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/scoop"
	"github.com/Tijani127/codenv/internal/store"
	"github.com/Tijani127/codenv/internal/ui"
)

type Engine struct {
	Store   *store.Store
	Runner  *scoop.Runner
	Catalog *manifest.Catalog
	Index   *store.Index
	Arch    string
}

func New(st *store.Store) (*Engine, error) {
	ix, err := store.LoadIndex(st)
	if err != nil {
		return nil, err
	}
	return &Engine{
		Store:   st,
		Runner:  scoop.New(st),
		Catalog: manifest.New(st, st.Arch()),
		Index:   ix,
		Arch:    st.Arch(),
	}, nil
}

func (e *Engine) Bootstrap(extra []string) error {
	buckets := store.DefaultBuckets()
	for _, b := range extra {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if _, ok := store.BucketRepo(b); !ok {
			return fmt.Errorf("unknown bucket %q", b)
		}
		buckets = append(buckets, b)
	}
	return e.Store.EnsureBuckets(dedupe(buckets))
}

func (e *Engine) SyncBuckets() error {
	return e.Store.SyncBuckets()
}

func (e *Engine) Resolve(specs []pkgspec.Spec) (*config.Lock, error) {
	lock := config.NewLock()
	resolved := map[string]bool{}
	visiting := map[string]bool{}

	var visit func(s pkgspec.Spec) (string, error)
	visit = func(s pkgspec.Spec) (string, error) {
		m, err := e.Catalog.Find(s)
		if err != nil {
			if s.Version == "" {
				return "", fmt.Errorf("resolving %s: %w", s.String(), err)
			}
			key := pkgspec.Spec{Bucket: s.Bucket, Name: s.Name}.Key(s.Version)
			if !resolved[key] {
				lock.Resolved[key] = config.LockedPackage{
					Bucket:       s.Bucket,
					Name:         s.Name,
					Version:      s.Version,
					Architecture: e.Arch,
					Unresolved:   true,
				}
				resolved[key] = true
			}
			return key, nil
		}
		version := m.Version
		key := pkgspec.Spec{Bucket: m.Bucket, Name: s.Name}.Key(version)

		if resolved[key] {
			return key, nil
		}
		if visiting[key] {
			return key, nil
		}
		visiting[key] = true
		defer delete(visiting, key)

		var depKeys []string
		for _, depName := range e.Catalog.Dependencies(m) {
			depSpec, err := pkgspec.Parse(depName)
			if err != nil {
				continue
			}
			depKey, err := visit(depSpec)
			if err != nil {
				return "", err
			}
			depKeys = append(depKeys, depKey)
		}

		lock.Resolved[key] = config.LockedPackage{
			Bucket:       m.Bucket,
			Name:         s.Name,
			Version:      version,
			Architecture: e.Arch,
			Hash:         m.CheckSHA256,
			Dependencies: dedupe(depKeys),
		}
		resolved[key] = true
		return key, nil
	}

	for _, s := range specs {
		key, err := visit(s)
		if err != nil {
			return nil, err
		}
		lock.Top = append(lock.Top, key)
	}
	return lock, nil
}

func (e *Engine) warnSuggestions(s pkgspec.Spec, suggestions []string) {
	const max = 6
	ui.Warn("no package named %q has version %s", s.Name, s.Version)
	if len(suggestions) == 0 {
		ui.Hint("run 'codenv search %s' to see what is available", s.Name)
		return
	}
	if len(suggestions) > 1 {
		shown := suggestions
		suffix := ""
		if len(shown) > max {
			shown = shown[:max]
			suffix = fmt.Sprintf(" (+%d more)", len(suggestions)-max)
		}
		ui.Info("matching packages: %s%s", strings.Join(shown, ", "), suffix)
	}
	ui.Hint("Scoop names versioned Python packages by minor, so try %q", suggestions[0])
}

type Replacement struct {
	Requested string
	Installed string
	Reason    string
}

type Report struct {
	Installed []string
	Reused    []string
}

func (e *Engine) Ensure(lock *config.Lock) (*Report, error) {
	report := &Report{}

	keys := make([]string, 0, len(lock.Resolved))
	for k := range lock.Resolved {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return rank(keys[i], lock) < rank(keys[j], lock) })

	var missing []string
	for _, k := range keys {
		pkg := lock.Resolved[k]
		if e.ready(pkg.Name, pkg.Version) {
			report.Reused = append(report.Reused, k)
			continue
		}
		missing = append(missing, k)
	}

	if len(missing) > 0 {
		var specs []string
		for _, k := range missing {
			specs = append(specs, installSpec(lock.Resolved[k]))
		}
		ui.Step("Installing %d package(s)", len(specs))
		if err := e.Runner.Install(specs...); err != nil {
			e.explainUnresolvedPins(missing, lock)
			return report, err
		}
		if err := e.reloadIndex(); err != nil {
			return report, err
		}
	}

	var stillMissing []string
	for _, k := range keys {
		pkg := lock.Resolved[k]
		if !e.ready(pkg.Name, pkg.Version) {
			stillMissing = append(stillMissing, k)
		}
	}

	if len(stillMissing) > 0 {
		e.explainUnresolvedPins(stillMissing, lock)
		return report, &MissingError{Packages: stillMissing, Lock: lock}
	}

	e.reconcile(lock, keys, report)
	return report, nil
}

func (e *Engine) reconcile(lock *config.Lock, keys []string, report *Report) {
	keyMap := map[string]string{}
	newResolved := make(map[string]config.LockedPackage, len(keys))

	for _, k := range keys {
		pkg := lock.Resolved[k]
		if pkg.Unresolved {
			pkg.Unresolved = false
		}
		if pkg.Bucket == "" {
			pkg.Bucket = e.bucketFor(pkg.Name)
		}
		newKey := pkgspec.Spec{Bucket: pkg.Bucket, Name: pkg.Name}.Key(pkg.Version)
		keyMap[k] = newKey
		newResolved[newKey] = pkg
		report.Installed = append(report.Installed, k)
	}

	for newKey, pkg := range newResolved {
		if len(pkg.Dependencies) == 0 {
			continue
		}
		remapped := make([]string, 0, len(pkg.Dependencies))
		for _, dep := range pkg.Dependencies {
			if mapped, ok := keyMap[dep]; ok {
				remapped = append(remapped, mapped)
				continue
			}
			remapped = append(remapped, dep)
		}
		pkg.Dependencies = dedupe(remapped)
		newResolved[newKey] = pkg
	}

	lock.Resolved = newResolved
	newTop := make([]string, 0, len(lock.Top))
	seen := map[string]bool{}
	for _, k := range lock.Top {
		mapped, ok := keyMap[k]
		if !ok {
			mapped = k
		}
		if seen[mapped] {
			continue
		}
		seen[mapped] = true
		newTop = append(newTop, mapped)
	}
	lock.Top = newTop
}

func (e *Engine) ready(name, version string) bool {
	if name == "" || version == "" {
		return false
	}
	if e.Store == nil || !e.Store.HasVersion(name, version) {
		return false
	}
	if e.Index.Has(name, version) {
		return true
	}
	pkg := config.LockedPackage{Name: name, Version: version}
	if pkg.Bucket == "" {
		pkg.Bucket = e.bucketFor(name)
	}
	return len(e.PathsForPkg(pkg)) > 0
}

func (e *Engine) anyInstalled(name string) (string, bool) {
	for _, v := range e.Store.InstalledVersions(name) {
		if e.ready(name, v) {
			return v, true
		}
	}
	return "", false
}

func (e *Engine) allPresent(keys []string, lock *config.Lock) bool {
	for _, k := range keys {
		pkg := lock.Resolved[k]
		if e.ready(pkg.Name, pkg.Version) {
			continue
		}
		if _, ok := e.anyInstalled(pkg.Name); !ok {
			return false
		}
	}
	return true
}

type MissingError struct {
	Packages []string
	Lock     *config.Lock
}

func (m *MissingError) Error() string {
	var names []string
	for _, k := range m.Packages {
		if pkg, ok := m.Lock.Resolved[k]; ok {
			names = append(names, pkg.String())
		}
	}
	return fmt.Sprintf(
		"could not install the exact versions this project pins: %s\n"+
			"codenv will not substitute a different version, because that would break reproducibility",
		strings.Join(names, ", "))
}

func (e *Engine) explainUnresolvedPins(keys []string, lock *config.Lock) {
	seen := map[string]bool{}
	for _, k := range keys {
		pkg, ok := lock.Resolved[k]
		if !ok || !pkg.Unresolved || seen[pkg.Name] {
			continue
		}
		seen[pkg.Name] = true
		spec := pkgspec.Spec{Bucket: pkg.Bucket, Name: pkg.Name, Version: pkg.Version}
		e.warnSuggestions(spec, e.Catalog.Suggest(pkg.Name, pkg.Version))
	}
}

func (e *Engine) bucketFor(name string) string {
	for _, b := range e.Catalog.BucketOrder() {
		if _, err := e.Catalog.Find(pkgspec.Spec{Bucket: b, Name: name}); err == nil {
			return b
		}
	}
	return ""
}

func rank(key string, lock *config.Lock) int {
	for i, k := range lock.Top {
		if k == key {
			return i
		}
	}
	return len(lock.Top) + 1
}

func installSpec(p config.LockedPackage) string {
	name := p.Name
	if p.Bucket != "" {
		name = p.Bucket + "/" + p.Name
	}
	if p.Version != "" {
		return name + "@" + p.Version
	}
	return name
}

func (e *Engine) reloadIndex() error {
	ix, err := store.LoadIndex(e.Store)
	if err != nil {
		return err
	}
	e.Index = ix
	return nil
}

func (e *Engine) Purge(apps []string) error {
	if len(apps) == 0 {
		return nil
	}
	if err := e.Runner.Uninstall(apps); err != nil {
		return err
	}
	for _, app := range apps {
		for _, v := range e.Store.InstalledVersions(app) {
			e.Index.Remove(app, v)
		}
	}
	return e.Index.Save()
}

func (e *Engine) PathsForPkg(p config.LockedPackage) []string {
	dirs := existingDirs(e.Index.Dirs(p.Name, p.Version))
	if len(dirs) > 0 {
		return dirs
	}
	if dirs := e.manifestDirs(p); len(dirs) > 0 {
		return dirs
	}
	if root := e.Store.VersionDir(p.Name, p.Version); isDir(root) {
		return []string{root}
	}
	return nil
}

func (e *Engine) manifestDirs(p config.LockedPackage) []string {
	m, err := e.Catalog.Find(pkgspec.Spec{Bucket: p.Bucket, Name: p.Name, Version: p.Version})
	if err != nil {
		return nil
	}
	base := e.Store.VersionDir(p.Name, p.Version)
	var out []string
	for _, entry := range m.BinEntries() {
		rel := strings.TrimPrefix(strings.TrimSpace(entry), "./")
		if rel == "" {
			continue
		}
		dir := filepath.Dir(filepath.Join(base, filepath.FromSlash(rel)))
		if isDir(dir) {
			out = append(out, dir)
		}
	}
	return existingDirs(out)
}

func (e *Engine) PathsFor(lock *config.Lock) []string {
	var out []string
	for _, key := range e.WalkOrder(lock) {
		out = append(out, e.PathsForPkg(lock.Resolved[key])...)
	}
	return dedupeDirs(out)
}

func (e *Engine) WalkOrder(lock *config.Lock) []string {
	seen := map[string]bool{}
	var order []string
	var visit func(key string)
	visit = func(key string) {
		if seen[key] {
			return
		}
		seen[key] = true
		order = append(order, key)
		if pkg, ok := lock.Resolved[key]; ok {
			for _, dep := range pkg.Dependencies {
				visit(dep)
			}
		}
	}
	for _, k := range lock.Top {
		visit(k)
	}
	return order
}

func existingDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if isDir(d) {
			out = append(out, filepath.Clean(d))
		}
	}
	return dedupeDirs(out)
}

func dedupeDirs(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		k := strings.ToLower(filepath.Clean(v))
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

func isDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
