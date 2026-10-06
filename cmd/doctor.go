package cmd

import (
	"fmt"
	"strings"

	"github.com/Tijani127/codenv/internal/doctor"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagDoctorJSON  bool
	flagDoctorQuiet bool
	flagDoctorFix   bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the codenv setup",
	Long: "Check that codenv can actually do its job on this machine.\n\n" +
		"Looks for the things that commonly go wrong: a missing or unwritable store, dirty\n" +
		"bucket manifests left behind by autoupdate, a leftover shims directory, stale index\n" +
		"entries, pinned versions that are not installed, and unset secret sources.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()

		var opts doctor.Options
		proj, projErr := currentProject(false)
		if projErr == nil {
			opts.Config = proj.Config
			opts.Lock = proj.Lock
			opts.ProjectDir = proj.Dir
			if st, err := proj.OpenStore(); err == nil {
				opts.Store = st
			}
		} else {
			opts.Store = nil
		}

		if flagDoctorFix {
			res, err := doctor.Fix(opts)
			if err != nil {
				return fail(1, "%s", err)
			}
			if res.Any() {
				var parts []string
				if res.RemovedShims > 0 {
					parts = append(parts, fmt.Sprintf("removed %d shim file(s)", res.RemovedShims))
				}
				if len(res.ResetBuckets) > 0 {
					parts = append(parts, "reset bucket(s) "+strings.Join(res.ResetBuckets, ", "))
				}
				if res.PrunedIndex > 0 {
					parts = append(parts, fmt.Sprintf("pruned %d stale index entries", res.PrunedIndex))
				}
				ui.Success("Fixed: %s", strings.Join(parts, "; "))
				ui.Println("")
			}
		}

		report := doctor.Run(opts)

		if flagDoctorJSON {
			emitDoctorJSON(report)
		} else {
			printDoctorReport(report, projErr != nil)
		}

		switch report.Worst() {
		case doctor.Fail:
			return &exitError{code: 1, err: nil}
		}
		return nil
	},
}

func printDoctorReport(r *doctor.Report, noProject bool) {
	width := 0
	for _, c := range r.Checks {
		if len(c.Name) > width {
			width = len(c.Name)
		}
	}
	for _, c := range r.Checks {
		label, style := "ok", ui.Green
		switch c.Level {
		case doctor.Warn:
			label, style = "warn", ui.Yellow
		case doctor.Fail:
			label, style = "fail", ui.LightRed
		}
		ui.Println("%-*s  %s  %s", width, c.Name, ui.Paint(style, label), c.Detail)
		if c.Hint != "" && !flagDoctorQuiet {
			ui.Println("%s%s", strings.Repeat(" ", width+9), ui.Paint(ui.Gray, c.Hint))
		}
	}
	if noProject && !flagDoctorQuiet {
		ui.Println("")
		ui.Info("no codenv.json found here, so project checks were skipped")
	}
	ui.Println("")
	switch r.Worst() {
	case doctor.Fail:
		ui.Errln("%d problem(s) will stop codenv from working", r.Count(doctor.Fail))
	case doctor.Warn:
		ui.Outln("%d warning(s); codenv works, but these are worth fixing", r.Count(doctor.Warn))
	default:
		ui.Success("everything looks healthy")
	}
}

func emitDoctorJSON(r *doctor.Report) {
	ui.Println(`{"checks":[`)
	for i, c := range r.Checks {
		comma := ","
		if i == len(r.Checks)-1 {
			comma = ""
		}
		hint := ""
		if c.Hint != "" {
			hint = `,"hint":` + jsonQuote(c.Hint)
		}
		ui.Println(`  {"name":%s,"status":%s,"detail":%s%s}%s`,
			jsonQuote(c.Name), jsonQuote(string(c.Level)), jsonQuote(c.Detail), hint, comma)
	}
	ui.Println(`],"status":%s}`, jsonQuote(string(r.Worst())))
}

func jsonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func init() {
	doctorCmd.Flags().BoolVar(&flagDoctorJSON, "json", false, "emit JSON")
	doctorCmd.Flags().BoolVar(&flagDoctorQuiet, "no-hints", false, "omit hints")
	doctorCmd.Flags().BoolVar(&flagDoctorFix, "fix", false, "repair what codenv can repair safely")
	rootCmd.AddCommand(doctorCmd)
}
