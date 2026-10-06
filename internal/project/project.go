package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/store"
)

type Project struct {
	Dir        string
	ConfigPath string
	Config     *config.Config
	LockPath   string
	Lock       *config.Lock
	Global     bool
	includes   []string
}

func Find(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, config.FileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func Load(dir string) (*Project, error) {
	cfgPath := filepath.Join(dir, config.FileName)
	cfg, err := loadWithIncludes(cfgPath)
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, config.LockName)
	lock, err := config.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	return &Project{
		Dir:        dir,
		ConfigPath: cfgPath,
		Config:     cfg,
		LockPath:   lockPath,
		Lock:       lock,
	}, nil
}

func loadWithIncludes(path string) (*config.Config, error) {
	seen := map[string]bool{}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	seen[resolved] = true
	return loadRecursive(path, seen)
}

func loadRecursive(path string, seen map[string]bool) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	merged := config.NewConfig()
	for _, inc := range cfg.Include {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(base, incPath)
		}
		abs, err := filepath.Abs(incPath)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			abs = filepath.Join(abs, config.FileName)
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		included, err := loadRecursive(abs, seen)
		if err != nil {
			return nil, err
		}
		merged = merged.Merge(included)
	}
	return merged.Merge(cfg), nil
}

func GlobalDir() (string, error) {
	home, err := store.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "global"), nil
}

func LoadGlobal() (*Project, error) {
	dir, err := GlobalDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dir, config.FileName)
	cfg := config.NewConfig()
	if _, err := os.Stat(cfgPath); err == nil {
		cfg, err = loadWithIncludes(cfgPath)
		if err != nil {
			return nil, err
		}
	}
	lockPath := filepath.Join(dir, config.LockName)
	lock, err := config.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	return &Project{
		Dir:        dir,
		ConfigPath: cfgPath,
		Config:     cfg,
		LockPath:   lockPath,
		Lock:       lock,
		Global:     true,
	}, nil
}

func (p *Project) StateDir() string {
	if p.Global {
		return filepath.Join(p.Dir, ".state")
	}
	return filepath.Join(p.Dir, ".codenv")
}

func (p *Project) StoreMode() store.Mode {
	if p.Global {
		return store.ModeGlobal
	}
	if p.Config.StoreMode() == "project" {
		return store.ModeProject
	}
	return store.ModeShared
}

func (p *Project) OpenStore() (*store.Store, error) {
	return store.Open(p.StoreMode(), p.Dir)
}

func (p *Project) Save() error {
	return config.Save(p.ConfigPath, p.Config)
}

func (p *Project) SaveLock() error {
	return config.SaveLock(p.LockPath, p.Lock)
}

func (p *Project) Exists() bool {
	_, err := os.Stat(p.ConfigPath)
	return err == nil
}

func (p *Project) Name() string {
	if p.Global {
		return "global"
	}
	return filepath.Base(p.Dir)
}

func ResolveConfigDir(flagValue string) (string, error) {
	if flagValue != "" {
		abs, err := filepath.Abs(flagValue)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(abs)
		if err == nil && !info.IsDir() {
			abs = filepath.Dir(abs)
		}
		if _, err := os.Stat(filepath.Join(abs, config.FileName)); err != nil {
			return "", fmt.Errorf("no %s found in %s", config.FileName, abs)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err := Find(cwd)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", fmt.Errorf("no %s found in %s or any parent directory\nrun 'codenv init' to create one", config.FileName, cwd)
	}
	return dir, nil
}

func RelOrSame(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return target
	}
	return rel
}
