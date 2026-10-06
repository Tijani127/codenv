package services

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tijani127/codenv/internal/shellrun"
)

const FileName = "codenv-services.json"

type Service struct {
	Command     string   `json:"command"`
	Environment []string `json:"environment"`
	WorkingDir  string   `json:"working_dir"`
	IsDaemon    bool     `json:"is_daemon"`
	DependsOn   []string `json:"depends_on"`
	Label       string   `json:"label"`
	Alias       string   `json:"alias"`
}

type Config struct {
	Services map[string]Service `json:"services"`
}

type Runtime struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	Log       string `json:"log"`
	Command   string `json:"command"`
	Daemon    bool   `json:"daemon"`
}

type State struct {
	Services map[string]Runtime `json:"services"`
}

type Manager struct {
	ProjectDir string
	StateDir   string
	Env        []string
	Config     Config
	PathExists bool
}

func Path(projectDir string) string {
	return filepath.Join(projectDir, FileName)
}

func Load(projectDir string, env []string) (*Manager, error) {
	m := &Manager{
		ProjectDir: projectDir,
		StateDir:   filepath.Join(projectDir, ".codenv", "services"),
		Env:        env,
		Config:     Config{Services: map[string]Service{}},
	}
	path := Path(projectDir)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &m.Config); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		m.PathExists = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if m.Config.Services == nil {
		m.Config.Services = map[string]Service{}
	}
	return m, nil
}

func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.Config.Services))
	for name := range m.Config.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Manager) resolve(requested []string) ([]string, error) {
	if len(requested) == 0 {
		if len(m.Config.Services) == 0 {
			return nil, fmt.Errorf("no services defined in %s", FileName)
		}
		return m.Names(), nil
	}
	var out []string
	for _, name := range requested {
		if _, ok := m.Config.Services[name]; !ok {
			return nil, fmt.Errorf("unknown service %q", name)
		}
		out = append(out, name)
	}
	return out, nil
}

func (m *Manager) statePath() string {
	return filepath.Join(m.StateDir, "state.json")
}

func (m *Manager) readState() *State {
	s := &State{Services: map[string]Runtime{}}
	if data, err := os.ReadFile(m.statePath()); err == nil {
		_ = json.Unmarshal(data, s)
		if s.Services == nil {
			s.Services = map[string]Runtime{}
		}
	}
	return s
}

func (m *Manager) writeState(s *State) error {
	if err := os.MkdirAll(m.StateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.statePath(), append(data, '\n'), 0o644)
}

func (m *Manager) logPath(name string) string {
	return filepath.Join(m.StateDir, sanitize(name)+".log")
}

func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (m *Manager) envFor(svc Service) []string {
	prefix := map[string]string{}
	if svc.WorkingDir != "" {
		prefix["CODEENV_SERVICE_DIR"] = svc.WorkingDir
	}
	for _, kv := range svc.Environment {
		if i := strings.Index(kv, "="); i > 0 {
			prefix[kv[:i]] = kv[i+1:]
		}
	}
	if len(prefix) == 0 {
		return m.Env
	}
	env := append([]string{}, m.Env...)
	keys := make([]string, 0, len(prefix))
	for k := range prefix {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+prefix[k])
	}
	return env
}

func (m *Manager) start(name string, background bool, out io.Writer) (int, error) {
	svc, ok := m.Config.Services[name]
	if !ok {
		return 0, fmt.Errorf("unknown service %q", name)
	}
	dir := m.ProjectDir
	if svc.WorkingDir != "" {
		dir = svc.WorkingDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(m.ProjectDir, dir)
		}
	}
	if err := os.MkdirAll(m.StateDir, 0o755); err != nil {
		return 0, err
	}
	cmd := shellrun.ScriptCommand(svc.Command)
	cmd.Dir = dir
	cmd.Env = m.envFor(svc)
	cmd.SysProcAttr = detachAttr()

	if background {
		logFile, err := os.OpenFile(m.logPath(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		defer logFile.Close()
		writeHeader(logFile, name, svc.Command)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		return cmd.Process.Pid, nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(m.logPath(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		writeHeader(logFile, name, svc.Command)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	pump := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		var pending strings.Builder
		for {
			n, err := r.Read(buf)
			if n > 0 {
				pending.Write(buf[:n])
				for {
					s := pending.String()
					idx := strings.IndexByte(s, '\n')
					if idx < 0 {
						break
					}
					line := s[:idx]
					pending.Reset()
					pending.WriteString(s[idx+1:])
					mu.Lock()
					fmt.Fprintf(out, "[%s] %s\n", name, line)
					if logFile != nil {
						fmt.Fprintf(logFile, "%s\n", line)
					}
					mu.Unlock()
				}
			}
			if err != nil {
				break
			}
		}
		if rest := pending.String(); rest != "" {
			mu.Lock()
			fmt.Fprintf(out, "[%s] %s\n", name, rest)
			if logFile != nil {
				fmt.Fprintf(logFile, "%s\n", rest)
			}
			mu.Unlock()
		}
	}
	wg.Add(2)
	go pump(stdout)
	go pump(stderr)
	go func() {
		_ = cmd.Wait()
		wg.Wait()
		if logFile != nil {
			logFile.Close()
		}
	}()
	return cmd.Process.Pid, nil
}

func writeHeader(w io.Writer, name, command string) {
	fmt.Fprintf(w, "=== codenv service %s ===\n=== command: %s ===\n=== started: %s ===\n",
		name, command, time.Now().Format(time.RFC3339))
}

func (m *Manager) Up(requested []string, background bool, out io.Writer) error {
	names, err := m.resolve(requested)
	if err != nil {
		return err
	}
	state := m.readState()
	ordered := m.orderByDependencies(names)
	var started []string
	for _, name := range ordered {
		if rt, ok := state.Services[name]; ok && Alive(rt.PID) {
			if background {
				fmt.Fprintf(out, "[%s] already running (pid %d)\n", name, rt.PID)
				continue
			}
		}
		pid, err := m.start(name, background, out)
		if err != nil {
			return fmt.Errorf("starting %s: %w", name, err)
		}
		state.Services[name] = Runtime{
			PID:       pid,
			StartedAt: time.Now().Format(time.RFC3339),
			Log:       m.logPath(name),
			Command:   m.Config.Services[name].Command,
			Daemon:    background,
		}
		started = append(started, name)
		if background {
			fmt.Fprintf(out, "[%s] started (pid %d)\n", name, pid)
		}
	}
	if err := m.writeState(state); err != nil {
		return err
	}
	if background {
		return nil
	}
	m.waitAll(ordered)
	return nil
}

func (m *Manager) orderByDependencies(names []string) []string {
	var out []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if svc, ok := m.Config.Services[name]; ok {
			deps := append([]string{}, svc.DependsOn...)
			if svc.Alias != "" {
				deps = append(deps, svc.Alias)
			}
			for _, dep := range deps {
				if _, ok := m.Config.Services[dep]; ok {
					visit(dep)
				}
			}
		}
		out = append(out, name)
	}
	for _, name := range names {
		visit(name)
	}
	return out
}

func (m *Manager) waitAll(names []string) {
	state := m.readState()
	for _, name := range names {
		rt, ok := state.Services[name]
		if !ok || rt.Daemon {
			continue
		}
		for Alive(rt.PID) {
			time.Sleep(250 * time.Millisecond)
		}
	}
}

func (m *Manager) Stop(requested []string, all bool) ([]string, error) {
	state := m.readState()
	targets := requested
	if all {
		targets = m.Names()
	}
	names, err := m.resolve(targets)
	if err != nil {
		return nil, err
	}
	var stopped []string
	for _, name := range names {
		rt, ok := state.Services[name]
		if !ok {
			continue
		}
		if Alive(rt.PID) {
			if err := kill(rt.PID); err != nil {
				return stopped, fmt.Errorf("stopping %s: %w", name, err)
			}
			waitGone(rt.PID, 5*time.Second)
		}
		delete(state.Services, name)
		stopped = append(stopped, name)
	}
	return stopped, m.writeState(state)
}

func waitGone(pid int, limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type Row struct {
	Name      string
	Command   string
	Status    string
	PID       int
	StartedAt string
	Daemon    bool
}

func (m *Manager) List() ([]Row, error) {
	state := m.readState()
	var rows []Row
	for _, name := range m.Names() {
		rt, ok := state.Services[name]
		row := Row{Name: name, Command: m.Config.Services[name].Command, Status: "stopped"}
		if ok {
			row.PID = rt.PID
			row.StartedAt = rt.StartedAt
			row.Daemon = rt.Daemon
			if Alive(rt.PID) {
				row.Status = "running"
			} else {
				row.Status = "exited"
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (m *Manager) Logs(requested []string) (map[string]string, error) {
	names, err := m.resolve(requested)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, name := range names {
		data, err := os.ReadFile(m.logPath(name))
		if err != nil {
			out[name] = ""
			continue
		}
		out[name] = string(data)
	}
	return out, nil
}

func (m *Manager) CleanLogs() error {
	entries, err := os.ReadDir(m.StateDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			_ = os.Remove(filepath.Join(m.StateDir, e.Name()))
		}
	}
	return nil
}
