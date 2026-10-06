package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Mode string

const (
	ModeShared  Mode = "shared"
	ModeProject Mode = "project"
	ModeGlobal  Mode = "global"
)

type Store struct {
	root string
	mode Mode
}

const (
	dirName      = ".codenv"
	configName   = "codenv.json"
	lockName     = "codenv.lock.json"
	indexName    = ".codenv-index.json"
	stateName    = ".codenv-state.json"
	scoopRepo    = "https://github.com/ScoopInstaller/Scoop"
	appDirName   = "apps"
	bucketDirNm  = "buckets"
	cacheDirName = "cache"
	shimsDirName = "shims"
	persistName  = "persist"
	currentLink  = "current"
)

func DefaultBuckets() []string {
	return []string{"main", "extras", "versions"}
}

func BucketRepo(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "main":
		return "https://github.com/ScoopInstaller/Main", true
	case "extras":
		return "https://github.com/ScoopInstaller/Extras", true
	case "versions":
		return "https://github.com/ScoopInstaller/Versions", true
	case "nirsoft":
		return "https://github.com/ScoopInstaller/Nirsoft", true
	case "sysinternals":
		return "https://github.com/ScoopInstaller/Sysinternals", true
	case "php":
		return "https://github.com/ScoopInstaller/PHP", true
	case "nerd-fonts":
		return "https://github.com/matthewjberger/scoop-nerd-fonts", true
	case "nonportable":
		return "https://github.com/ScoopInstaller/Nonportable", true
	case "java":
		return "https://github.com/ScoopInstaller/Java", true
	case "games":
		return "https://github.com/ScoopInstaller/Games", true
	default:
		return "", false
	}
}

func Home() (string, error) {
	if v := os.Getenv("CODEENV_HOME"); v != "" {
		return filepath.Abs(v)
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return filepath.Join(v, "codenv"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codenv"), nil
}

func SharedRoot() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "store"), nil
}

func GlobalRoot() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "global", "store"), nil
}

func ProjectRoot(projectDir string) string {
	return filepath.Join(projectDir, dirName, "store")
}

func Open(mode Mode, projectDir string) (*Store, error) {
	var root string
	var err error
	switch mode {
	case ModeProject:
		if projectDir == "" {
			return nil, fmt.Errorf("project store requires a project directory")
		}
		root = ProjectRoot(projectDir)
	case ModeGlobal:
		root, err = GlobalRoot()
	case ModeShared:
		root, err = SharedRoot()
	default:
		return nil, fmt.Errorf("unknown store mode %q", mode)
	}
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs, mode: mode}, nil
}

func (s *Store) Root() string    { return s.root }
func (s *Store) Mode() Mode      { return s.mode }
func (s *Store) AppsDir() string { return filepath.Join(s.root, appDirName) }
func (s *Store) BucketsDir() string {
	return filepath.Join(s.root, bucketDirNm)
}
func (s *Store) CacheDir() string { return filepath.Join(s.root, cacheDirName) }
func (s *Store) ShimsDir() string { return filepath.Join(s.root, shimsDirName) }
func (s *Store) PersistDir() string {
	return filepath.Join(s.root, persistName)
}
func (s *Store) IndexPath() string {
	return filepath.Join(s.root, indexName)
}
func (s *Store) ScoopAppDir() string {
	return filepath.Join(s.AppsDir(), "scoop", currentLink)
}
func (s *Store) ScoopScript() string {
	return filepath.Join(s.ScoopAppDir(), "bin", "scoop.ps1")
}

func (s *Store) AppDir(app string) string {
	return filepath.Join(s.AppsDir(), app)
}

func (s *Store) VersionDir(app, version string) string {
	return filepath.Join(s.AppsDir(), app, version)
}

func (s *Store) HasVersion(app, version string) bool {
	if version == "" {
		return false
	}
	info, err := os.Stat(s.VersionDir(app, version))
	return err == nil && info.IsDir()
}

func (s *Store) CurrentPath(app string) string {
	return filepath.Join(s.AppDir(app), currentLink)
}

func (s *Store) ResolveCurrentVersion(app string) (string, error) {
	target, err := os.Readlink(s.CurrentPath(app))
	if err != nil {
		return "", err
	}
	return filepath.Base(strings.TrimRight(target, `\/`)), nil
}

func (s *Store) InstalledVersions(app string) []string {
	entries, err := os.ReadDir(s.AppDir(app))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == currentLink {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

func (s *Store) InstalledApps() []string {
	entries, err := os.ReadDir(s.AppsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *Store) Buckets() []string {
	entries, err := os.ReadDir(s.BucketsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *Store) HasBucket(name string) bool {
	info, err := os.Stat(filepath.Join(s.BucketsDir(), name))
	return err == nil && info.IsDir()
}

func (s *Store) Arch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "64bit"
}

func (s *Store) EnsureDirs() error {
	for _, d := range []string{
		s.root,
		s.AppsDir(),
		s.BucketsDir(),
		s.CacheDir(),
		s.PersistDir(),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Ensure() error {
	if err := s.EnsureDirs(); err != nil {
		return err
	}
	if err := s.ensureScoopCode(); err != nil {
		return err
	}
	return s.ensureBuckets(DefaultBuckets())
}

func (s *Store) EnsureBuckets(names []string) error {
	if err := s.EnsureDirs(); err != nil {
		return err
	}
	if err := s.ensureScoopCode(); err != nil {
		return err
	}
	return s.ensureBuckets(names)
}

func (s *Store) ensureScoopCode() error {
	if _, err := os.Stat(s.ScoopScript()); err == nil {
		return nil
	}
	shared, err := SharedRoot()
	if err != nil {
		return err
	}
	donor := filepath.Join(shared, appDirName, "scoop", currentLink)
	donorScript := filepath.Join(donor, "bin", "scoop.ps1")
	if s.root != shared {
		if _, err := os.Stat(donorScript); err == nil {
			if err := os.MkdirAll(filepath.Dir(s.ScoopAppDir()), 0o755); err != nil {
				return err
			}
			if err := linkJunction(donor, s.ScoopAppDir()); err == nil {
				return nil
			}
		}
	}
	return CloneRepo(scoopRepo, s.ScoopAppDir(), "master")
}

func (s *Store) ensureBuckets(names []string) error {
	for _, name := range names {
		if s.HasBucket(name) {
			continue
		}
		repo, ok := BucketRepo(name)
		if !ok {
			return fmt.Errorf("unknown bucket %q", name)
		}
		if err := CloneRepo(repo, filepath.Join(s.BucketsDir(), name), "master"); err != nil {
			return fmt.Errorf("clone bucket %s: %w", name, err)
		}
	}
	return nil
}

func CloneRepo(url, dest, branch string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required to bootstrap codenv: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", branch, url, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone %s: %w\n%s", url, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Store) SyncBuckets() error {
	for _, name := range s.Buckets() {
		dir := filepath.Join(s.BucketsDir(), name)
		cmd := exec.Command("git", "-C", dir, "pull", "--ff-only", "--quiet")
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("pull bucket %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func (s *Store) ResetBuckets() {
	for _, name := range s.Buckets() {
		dir := filepath.Join(s.BucketsDir(), name)
		if !s.bucketIsGitRepo(dir) {
			continue
		}
		cmd := exec.Command("git", "-C", dir, "checkout", "--", ".")
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		_ = cmd.Run()
	}
}

func (s *Store) bucketIsGitRepo(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, ".git")); err != nil || !info.IsDir() {
		return false
	}
	return true
}

func linkJunction(target, link string) error {
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mklink: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
