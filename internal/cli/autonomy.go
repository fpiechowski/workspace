package cli

import (
	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func autonomyCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "autonomy", Short: "Manage the per-workspace autonomous run"}

	var enable core.AutonomyEnableOptions
	enableCmd := command("enable", "Enable autonomous mode from the user terminal", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.EnableAutonomy(c.Context(), id, enable, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	enableCmd.Flags().StringVar(&enable.Reason, "reason", "", "Why autonomous mode is being enabled")
	enableCmd.Flags().IntVar(&enable.ExpectedRevision, "expected-revision", 0, "Required current workspace revision")
	group.AddCommand(enableCmd)

	var disable core.AutonomyDisableOptions
	disableCmd := command("disable", "Disable autonomous mode (user, or orchestrator with --user-confirmed)", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.DisableAutonomy(c.Context(), id, disable, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	disableCmd.Flags().StringVar(&disable.Reason, "reason", "", "Why autonomous mode is being disabled")
	disableCmd.Flags().IntVar(&disable.ExpectedRevision, "expected-revision", 0, "Required current workspace revision")
	disableCmd.Flags().BoolVar(&disable.UserConfirmed, "user-confirmed", false, "Attest that the user requested disabling autonomy")
	group.AddCommand(disableCmd)

	var report core.AutonomyReportOptions
	reportCmd := command("report", "Deliver the autonomous final report and end the run", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.ReportAutonomy(c.Context(), id, report, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	reportCmd.Flags().StringVar(&report.Outcome, "outcome", "", "ready_to_land | ready_to_complete | blocked | failed")
	reportCmd.Flags().StringVar(&report.Recommendation, "recommendation", "", "One-paragraph recommendation shown to the user")
	reportCmd.Flags().StringVar(&report.SummaryFile, "summary-file", "", "Markdown summary stored as an immutable report artifact")
	reportCmd.Flags().StringSliceVar(&report.Artifacts, "artifact", nil, "Additional report artifact; repeat or comma-separate")
	reportCmd.Flags().StringSliceVar(&report.Pending, "pending", nil, "Pending item for a blocked/failed report; repeat or comma-separate")
	reportCmd.Flags().IntVar(&report.ExpectedRevision, "expected-revision", 0, "Required current workspace revision")
	group.AddCommand(reportCmd)

	return group
}
