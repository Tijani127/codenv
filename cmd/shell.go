package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/envbuild"
	"github.com/Tijani127/codenv/internal/shellrun"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

type envFlags struct {
	env       []string
	envFile   string
	pure      bool
	format    string
	noInstall bool
	noBanner  bool
	noPrompt  bool
}

func (f *envFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringArrayVarP(&f.env, "env", "e", nil, "environment variable to set (KEY=VALUE)")
	cmd.Flags().StringVar(&f.envFile, "env-file", "", "path to a file of environment variables")
	cmd.Flags().BoolVar(&f.pure, "pure", false, "inherit almost nothing from the current environment")
	cmd.Flags().StringVar(&f.format, "format", "auto", "shell format for printed env (powershell, bash, cmd)")
	cmd.Flags().BoolVar(&f.noInstall, "no-install", false, "fail instead of installing missing packages")
	cmd.Flags().BoolVar(&f.noBanner, "no-banner", false, "do not print the codenv banner")
}

func (f *envFlags) registerNoPrompt(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.noPrompt, "no-prompt", false, "do not mark the shell prompt while inside codenv")
}

func (f *envFlags) options() (envbuild.Options, error) {
	format, err := envbuild.ParseFormat(f.format)
	if err != nil {
		return envbuild.Options{}, fail(2, "%s", err)
	}
	if f.format == "" || f.format == "auto" {
		format = envbuild.DetectFormat()
	}
	opts := envbuild.Options{
		Pure:      f.pure,
		Format:    format,
		Overrides: splitEnvAssignments(f.env),
	}
	if f.envFile != "" {
		vars, err := readEnvFile(f.envFile)
		if err != nil {
			return opts, err
		}
		opts.EnvFile = vars
	}
	return opts, nil
}

var shellFlags envFlags
var shellPromptMark = true

var shellCmd = &cobra.Command{
	Use:   "shell [command...]",
	Short: "Start a shell with the project's packages on PATH",
	Long: "Start an interactive shell, or run a single command, inside the codenv environment.\n\n" +
		"Packages come from a shared store and are placed on PATH directly from their install\n" +
		"directories. Scoop shims are never created and your system PATH is never modified.",
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		opts, err := shellFlags.options()
		if err != nil {
			return err
		}
		sessOpts := sessionOpts{}
		if shellFlags.noInstall {
			sessOpts.SkipInstall = true
		}
		sess, err := openSession(proj, sessOpts)
		if err != nil {
			return err
		}
		if err := sess.resolveSecrets(true); err != nil {
			return err
		}
		if shellFlags.noInstall {
			if err := verifyReady(sess); err != nil {
				return err
			}
		}
		env := sess.buildEnv(opts)
		childEnv := env.Apply(os.Environ())

		if len(args) > 0 {
			if err := runInitHook(childEnv, sess.Project.Dir, proj.Config.InitHook()); err != nil {
				return err
			}
			code, err := shellrun.Exec(childEnv, sess.Project.Dir, args[0], args[1:], os.Stdin, os.Stdout, os.Stderr)
			if err != nil {
				return fail(1, "%s", err)
			}
			if code != 0 {
				return &exitError{code: code, err: nil}
			}
			return nil
		}

		if !shellFlags.noBanner {
			printBanner(sess)
		}
		hook := proj.Config.InitHook()
		snippet := buildShellSnippet(hook, !shellPromptMark || shellFlags.noPrompt)
		if err := shellrun.Interactive(childEnv, sess.Project.Dir, snippet); err != nil {
			if ee, ok := err.(*shellrun.ExitError); ok {
				return &exitError{code: ee.Code, err: nil}
			}
			return fail(1, "%s", err)
		}
		return nil
	},
}

func runInitHook(env []string, dir, hook string) error {
	if hook == "" {
		return nil
	}
	code, err := shellrun.Script(env, dir, hook, nil, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return fail(1, "init_hook: %s", err)
	}
	if code != 0 {
		return fail(code, "init_hook exited with status %d", code)
	}
	return nil
}

func verifyReady(sess *session) error {
	var missing []string
	for _, key := range sess.Lock.Top {
		pkg, ok := sess.Lock.Resolved[key]
		if !ok {
			continue
		}
		if !sess.Store.HasVersion(pkg.Name, pkg.Version) {
			missing = append(missing, pkg.String())
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fail(2, "missing packages: %s\nrun 'codenv install' or drop --no-install", strings.Join(missing, ", "))
}

func printBanner(sess *session) {
	ui.Println("%s", ui.Paint(ui.LightCyan, "codenv")) //+" "+ui.Paint(ui.Gray, sess.banner()))
	if len(sess.Lock.Top) > 0 {
		names := make([]string, 0, len(sess.Lock.Top))
		for _, key := range sess.Lock.Top {
			if pkg, ok := sess.Lock.Resolved[key]; ok {
				names = append(names, pkg.Name)
			}
		}
		ui.Println("%s", ui.Paint(ui.Gray, "  "+strings.Join(names, "  ")))
	}
}

func buildShellSnippet(initHook string, noPrompt bool) string {
	shell := shellrun.Detect()
	var parts []string
	if initHook != "" {
		parts = append(parts, initHook)
	}
	if !noPrompt {
		switch shell.Kind {
		case shellrun.KindPowerShell:
			parts = append(parts,
				"$__codenv_prev_prompt = $function:prompt",
				"function global:prompt { \"[codenv] \" + (& $__codenv_prev_prompt) }",
				"$Host.UI.RawUI.WindowTitle = \"codenv - $PWD\"",
			)
		case shellrun.KindBash:
			parts = append(parts,
				`__codenv_prev_ps1="$PS1"`,
				`PS1="(codenv) $PS1"`,
			)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

var runFlags envFlags

var runCmd = &cobra.Command{
	Use:   "run <script|command> [args...]",
	Short: "Run a script or command inside the codenv environment",
	Long: "Run a script defined in codenv.json, or an arbitrary command, inside a codenv\n" +
		"environment. The environment exits when the command finishes.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		opts, err := runFlags.options()
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: runFlags.noInstall})
		if err != nil {
			return err
		}
		if err := sess.resolveSecrets(true); err != nil {
			return err
		}
		if runFlags.noInstall {
			if err := verifyReady(sess); err != nil {
				return err
			}
		}

		name, rest := splitRunArgs(args)
		env := sess.buildEnv(opts)
		childEnv := env.Apply(os.Environ())

		if hook := proj.Config.InitHook(); hook != "" {
			if err := runInitHook(childEnv, sess.Project.Dir, hook); err != nil {
				return err
			}
		}

		if script, ok := proj.Config.Script(name); ok {
			code, err := shellrun.Script(childEnv, sess.Project.Dir, script, rest, os.Stdin, os.Stdout, os.Stderr)
			if err != nil {
				return fail(1, "%s", err)
			}
			if code != 0 {
				return &exitError{code: code, err: nil}
			}
			return nil
		}

		if len(proj.Config.ScriptNames()) > 0 && looksLikeScript(name, proj.Config) {
			return fail(2, "script %q is not defined in %s", name, filepath.Base(proj.ConfigPath))
		}

		code, err := shellrun.Exec(childEnv, sess.Project.Dir, name, rest, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			return fail(127, "%s", err)
		}
		if code != 0 {
			return &exitError{code: code, err: nil}
		}
		return nil
	},
}

func splitRunArgs(args []string) (string, []string) {
	if idx := indexOf(args, "--"); idx >= 0 {
		return args[0], args[idx+1:]
	}
	return args[0], args[1:]
}

func indexOf(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}

func looksLikeScript(name string, cfg *config.Config) bool {
	for _, s := range cfg.ScriptNames() {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}

var shellenvFlags struct {
	format  string
	compact bool
}

var shellenvCmd = &cobra.Command{
	Use:   "shellenv",
	Short: "Print shell commands that add this project's packages to PATH",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := envbuild.ParseFormat(shellenvFlags.format)
		if err != nil {
			return fail(2, "%s", err)
		}
		if shellenvFlags.format == "" || shellenvFlags.format == "auto" {
			format = envbuild.DetectFormat()
		}
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true})
		if err != nil {
			return err
		}
		if err := sess.resolveSecrets(false); err != nil {
			return err
		}
		if err := verifyReady(sess); err != nil {
			return err
		}
		env := sess.buildEnv(envbuild.Options{Format: format})
		if shellenvFlags.compact {
			ui.Println("%s", env.Compact(format))
			return nil
		}
		ui.Print(env.Script(format))
		return nil
	},
}

func init() {
	shellCmd.Flags().SetInterspersed(false)
	runCmd.Flags().SetInterspersed(false)
	shellFlags.register(shellCmd)
	runFlags.register(runCmd)
	shellFlags.registerNoPrompt(shellCmd)
	runFlags.registerNoPrompt(runCmd)
	shellCmd.Flags().BoolVar(&shellPromptMark, "prompt", true, "mark the shell prompt while inside codenv")
	shellenvCmd.Flags().StringVar(&shellenvFlags.format, "format", "auto", "shell format to print (powershell, bash, cmd)")
	shellenvCmd.Flags().BoolVar(&shellenvFlags.compact, "compact", false, "print a single-line PATH export")
	rootCmd.AddCommand(shellCmd, runCmd, shellenvCmd)
}
