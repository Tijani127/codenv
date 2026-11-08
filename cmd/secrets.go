package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/project"
	"github.com/Tijani127/codenv/internal/secrets"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Declare which environment variables are secret",
	Long: "Manage the secret *declaration* in codenv.json.\n\n" +
		"By default codenv never stores secret values: you declare which variables a\n" +
		"project needs, and codenv picks their values up from your environment when\n" +
		"a shell or script starts.\n\n" +
		"With 'codenv secrets set <NAME>' a value can be stored in the Windows\n" +
		"Credential Manager (never on codenv's own disk) as a fallback for names\n" +
		"your environment does not provide.",
}

var secretsAddCmd = &cobra.Command{
	Use:   "add <NAME>",
	Short: "Declare a variable as a required secret",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		names := splitSecretArgs(args)
		s := proj.Config.EnsureSecrets()
		existing := s.Names
		for _, n := range names {
			if proj.Config.IsSecret(n) {
				ui.Warn("%s is already declared as a secret", n)
				continue
			}
			existing = append(existing, n)
		}
		sort.Strings(existing)
		s.Names = existing
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Declared %s as a required secret", strings.Join(names, ", "))
		ui.Hint("set it in your environment, then run 'codenv shell'")
		return nil
	},
}

var (
	flagSecretsFrom  string
	flagSecretsAllow bool
)

var (
	flagSecretsSetStdin bool
	flagSecretsRmStored bool
)

var secretsSetCmd = &cobra.Command{
	Use:   "set <NAME> [VALUE]",
	Short: "Store a secret value in the Windows Credential Manager",
	Long: "Store a secret's value in the Windows Credential Manager (target\n" +
		"codenv:secret:<NAME>) and declare NAME as a required secret.\n\n" +
		"The value is never written to codenv.json or the project directory. When a\n" +
		"shell or script resolves secrets, names not found in the environment fall\n" +
		"back to the Credential Manager.\n\n" +
		"Pass the value on stdin with --stdin to keep it out of shell history:\n" +
		" (Get-Content -Raw secret.txt) | codenv secrets set API_KEY --stdin",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		name := strings.ToUpper(strings.TrimSpace(args[0]))
		if name == "" {
			return fail(2, "a secret name is required")
		}
		var value string
		switch {
		case flagSecretsSetStdin:
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fail(1, "reading the value from stdin: %s", err)
			}
			// A piped file normally ends in exactly one newline.
			value = strings.TrimRight(string(data), "\r\n")
		case len(args) == 2:
			value = args[1]
		default:
			return fail(2, "a value is required (pass it on stdin with --stdin to keep it out of shell history)")
		}
		if value == "" {
			return fail(2, "the value is empty")
		}
		if err := secrets.StoreCredential(name, value); err != nil {
			return fail(1, "storing %s: %s", secrets.CredentialTarget(name), err)
		}
		s := proj.Config.EnsureSecrets()
		if !proj.Config.IsSecret(name) {
			s.Names = append(s.Names, name)
			sort.Strings(s.Names)
		}
		// A from-mapping would make the environment shadow the stored value.
		if proj.Config.HasSecretMapping(name) {
			delete(s.From, name)
			ui.Warn("%s had a from-mapping; it is now stored in the Credential Manager instead", name)
		}
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Stored %s in the Credential Manager", name)
		ui.Hint("shells read it when %s is not in the environment; remove it with 'codenv secrets rm %s --stored'", name, name)
		return nil
	},
}

var secretsFromCmd = &cobra.Command{
	Use:   "from <NAME> <ENV_VAR[,ENV_VAR...]>",
	Short: "Map a secret to one or more environment variables",
	Long: "Map a secret name to values taken from your environment.\n\n" +
		"Example:\n" +
		"  codenv secrets from DATABASE_URL DB_URL\n" +
		"  codenv secrets from API_KEY API_KEY,API_KEY_FALLBACK",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		name := strings.ToUpper(strings.TrimSpace(args[0]))
		source := strings.TrimSpace(args[1])
		if name == "" || source == "" {
			return fail(2, "both a secret name and a source variable are required")
		}
		s := proj.Config.EnsureSecrets()
		s.From[name] = source
		if !proj.Config.IsSecret(name) {
			s.Names = append(s.Names, name)
			sort.Strings(s.Names)
		}
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("%s now reads from %s", name, source)
		return nil
	},
}

var secretsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List declared secrets and whether their values are present",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		resolved, err := resolveFor(proj)
		if err != nil {
			return err
		}
		names := proj.Config.SecretNames()
		if len(names) == 0 {
			ui.Info("no secrets declared")
			ui.Hint("add one with 'codenv secrets add <NAME>'")
			return nil
		}
		width := len("NAME")
		for _, n := range names {
			if len(n) > width {
				width = len(n)
			}
		}
		ui.Println("%-*s  %-10s  %s", width, "NAME", "STATUS", "SOURCE")
		for _, n := range names {
			status := ui.Paint(ui.Yellow, "unset")
			source := proj.Config.SourceFor(n)
			if _, ok := resolved.Values[n]; ok {
				status = ui.Paint(ui.Green, "set")
			}
			switch {
			case resolved.Sources[n] == "credential manager":
				source = "credential manager"
			case source == "":
				source = n
			}
			ui.Println("%-*s  %s  %s", width, n, status, ui.Paint(ui.Gray, source))
		}
		return nil
	},
}

var secretsRmCmd = &cobra.Command{
	Use:   "rm <NAME>...",
	Short: "Stop treating variables as secrets (optionally delete stored values)",
	Long: "Remove names from the secret declaration in codenv.json.\n\n" +
		"With --stored, the values stored in the Windows Credential Manager under\n" +
		"codenv:secret:<NAME> are deleted as well.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		drop := map[string]bool{}
		for _, n := range splitSecretArgs(args) {
			drop[n] = true
		}
		var kept []string
		var gone []string
		for _, n := range proj.Config.SecretNames() {
			if drop[n] {
				gone = append(gone, n)
				continue
			}
			kept = append(kept, n)
		}
		for n := range drop {
			if proj.Config.HasSecretMapping(n) {
				delete(proj.Config.EnsureSecrets().From, n)
				found := false
				for _, g := range gone {
					if g == n {
						found = true
					}
				}
				if !found {
					gone = append(gone, n)
				}
			}
		}
		if len(gone) == 0 {
			ui.Warn("nothing to remove")
			return nil
		}
		sort.Strings(gone)
		if flagSecretsRmStored {
			for _, n := range gone {
				_, err := secrets.DeleteCredential(n)
				if err != nil {
					ui.Warn("could not delete the Credential Manager entry for %s: %s", n, err)
					continue
				}
				ui.Detail("deleted the Credential Manager entry for %s", n)
			}
		}
		if len(kept) == 0 && len(gone) > 0 {
			proj.Config.Secrets = nil
		} else if proj.Config.Secrets != nil {
			proj.Config.Secrets.Names = kept
		}
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Removed %s", strings.Join(gone, ", "))
		return nil
	},
}

var (
	flagDownloadFormat string
	flagDownloadValues bool
)

var secretsDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Write resolved secrets to a file or stdout",
	Long: "Write the values of your declared secrets to a .env file or to stdout.\n\n" +
		"The output is redacted by default; pass --values to include real values.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		resolved, err := resolveFor(proj)
		if err != nil {
			return err
		}
		if err := resolved.Report(); err != nil {
			return fail(2, "%s", err)
		}
		var b strings.Builder
		for _, n := range sortedKeys(resolved.Values) {
			if flagDownloadValues {
				b.WriteString(fmt.Sprintf("%s=%s\n", n, resolved.Values[n]))
				continue
			}
			value := "***redacted***"
			if _, ok := resolved.Values[n]; ok {
				b.WriteString(fmt.Sprintf("%s=%s\n", n, value))
			} else {
				b.WriteString(fmt.Sprintf("# %s is not set in your environment\n", n))
			}
		}
		switch flagDownloadFormat {
		case "json":
			ui.Println("%s", b.String())
		default:
			ui.Print(b.String())
		}
		return nil
	},
}

func resolveFor(proj *project.Project) (*secrets.Resolved, error) {
	resolved, err := secrets.ResolveFromEnv(proj.Config)
	if err != nil {
		return nil, fail(2, "%s", err)
	}
	return resolved, nil
}

func splitSecretArgs(args []string) []string {
	var out []string
	for _, a := range args {
		for _, part := range strings.Split(a, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	secretsFromCmd.Flags().BoolVar(&flagSecretsAllow, "allow-missing", false, "tolerate source variables that are unset")
	secretsDownloadCmd.Flags().StringVar(&flagDownloadFormat, "format", "env", "output format (env, json)")
	secretsDownloadCmd.Flags().BoolVar(&flagDownloadValues, "values", false, "include real values instead of redacting")
	secretsSetCmd.Flags().BoolVar(&flagSecretsSetStdin, "stdin", false, "read the value from stdin instead of an argument")
	secretsRmCmd.Flags().BoolVar(&flagSecretsRmStored, "stored", false, "also delete values stored in the Windows Credential Manager")
	secretsCmd.AddCommand(secretsAddCmd, secretsFromCmd, secretsListCmd, secretsRmCmd, secretsDownloadCmd, secretsSetCmd)
	rootCmd.AddCommand(secretsCmd)
}
