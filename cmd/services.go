package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/envbuild"
	"github.com/Tijani127/codenv/internal/services"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var servicesCmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"svc"},
	Short:   "Run the project's background services",
	Long: "Manage services declared in " + services.FileName + ".\n\n" +
		"codenv supervises these natively, so no extra process manager is required on Windows.\n" +
		"Each service inherits the codenv environment, so packages from codenv.json are on PATH.",
}

func newServiceManager(cmd *cobra.Command, useGlobal bool) (*services.Manager, *session, error) {
	proj, err := currentProject(useGlobal)
	if err != nil {
		return nil, nil, err
	}
	sess, err := openSession(proj, sessionOpts{})
	if err != nil {
		return nil, nil, err
	}
	opts := envbuild.Options{Format: envbuild.DetectFormat()}
	env := sess.buildEnv(opts)
	mgr, err := services.Load(proj.Dir, env.Apply(os.Environ()))
	if err != nil {
		return nil, nil, fail(1, "%s", err)
	}
	return mgr, sess, nil
}

var servicesUpFlags struct {
	background bool
	purgeLogs  bool
}

var servicesUpCmd = &cobra.Command{
	Use:   "up [service...]",
	Short: "Start the services and stream their logs",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		if servicesUpFlags.purgeLogs {
			_ = mgr.CleanLogs()
		}
		if servicesUpFlags.background {
			return mgr.Up(args, true, os.Stdout)
		}
		ui.Step("Starting %s", strings.Join(mgr.Names(), ", "))
		if err := mgr.Up(args, false, os.Stdout); err != nil {
			return fail(1, "%s", err)
		}
		return nil
	},
}

var servicesStartCmd = &cobra.Command{
	Use:   "start [service...]",
	Short: "Start services in the background",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		return mgr.Up(args, true, os.Stdout)
	},
}

var servicesStopCmd = &cobra.Command{
	Use:   "stop [service...]",
	Short: "Stop running services",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		stopped, err := mgr.Stop(args, false)
		if err != nil {
			return fail(1, "%s", err)
		}
		if len(stopped) == 0 {
			ui.Info("nothing to stop")
			return nil
		}
		ui.Success("Stopped %s", strings.Join(stopped, ", "))
		return nil
	},
}

var servicesRestartCmd = &cobra.Command{
	Use:   "restart [service...]",
	Short: "Restart services",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		if _, err := mgr.Stop(args, false); err != nil {
			return fail(1, "%s", err)
		}
		ui.Step("Restarting")
		return mgr.Up(args, true, os.Stdout)
	},
}

var servicesLsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list", "ps"},
	Short:   "List services and their status",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		rows, err := mgr.List()
		if err != nil {
			return fail(1, "%s", err)
		}
		if len(rows) == 0 {
			ui.Info("no services defined in %s", services.FileName)
			ui.Hint(`create one with {"services": {"api": {"command": "npm run dev"}}}`)
			return nil
		}
		width := len("SERVICE")
		for _, r := range rows {
			if len(r.Name) > width {
				width = len(r.Name)
			}
		}
		ui.Println("%-*s  %-9s  %-8s  %-21s  %s", width, "SERVICE", "STATUS", "PID", "STARTED", "COMMAND")
		for _, r := range rows {
			style := ui.Gray
			switch r.Status {
			case "running":
				style = ui.Green
			case "exited":
				style = ui.Yellow
			}
			pid := "-"
			if r.PID > 0 {
				pid = fmt.Sprint(r.PID)
			}
			ui.Println("%-*s  %s  %-8s  %-21s  %s",
				width, r.Name, ui.Paint(style, r.Status), pid, r.StartedAt, r.Command)
		}
		return nil
	},
}

var servicesLogsCmd = &cobra.Command{
	Use:     "logs [service...]",
	Aliases: []string{"attach"},
	Short:   "Print service logs",
	Args:    cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		logs, err := mgr.Logs(args)
		if err != nil {
			return fail(1, "%s", err)
		}
		names := make([]string, 0, len(logs))
		for name := range logs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ui.Println("%s", ui.Paint(ui.Bold, "=== "+name+" ==="))
			ui.Print(logs[name])
			if !strings.HasSuffix(logs[name], "\n") {
				ui.Println("")
			}
		}
		return nil
	},
}

var servicesCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Stop every service and delete their logs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := newServiceManager(cmd, false)
		if err != nil {
			return err
		}
		stopped, err := mgr.Stop(nil, true)
		if err != nil {
			return fail(1, "%s", err)
		}
		_ = mgr.CleanLogs()
		ui.Success("Stopped %d service(s) and cleared logs", len(stopped))
		return nil
	},
}

func init() {
	servicesUpCmd.Flags().BoolVarP(&servicesUpFlags.background, "background", "b", false, "start in the background and return")
	servicesUpCmd.Flags().BoolVar(&servicesUpFlags.purgeLogs, "purge", false, "discard previous logs first")
	servicesCmd.AddCommand(
		servicesUpCmd,
		servicesStartCmd,
		servicesStopCmd,
		servicesRestartCmd,
		servicesLsCmd,
		servicesLogsCmd,
		servicesCleanCmd,
	)
	rootCmd.AddCommand(servicesCmd)
}
