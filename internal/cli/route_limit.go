package cli

import (
	"time"

	"github.com/spf13/cobra"

	"workspace/internal/core"
)

// routeLimitCommands exposes the project route-limit ledger under
// workspace profile limit.
func routeLimitCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "limit", Short: "Inspect and manage recorded route usage limits"}

	var all bool
	list := command("list", "List active route usage limits", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.ListRouteLimits(c.Context(), all)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	list.Flags().BoolVar(&all, "all", false, "Include cleared and expired records that are still retained")
	group.AddCommand(list)

	var set core.RouteLimitSetOptions
	var until string
	set.Kind = "quota_exhausted"
	setCmd := command("set <target>", "Record a manual usage limit for a route", func(c *cobra.Command, args []string) error {
		s, selector, err := o.optionalScope()
		if err != nil {
			return err
		}
		opt := set
		opt.Target = args[0]
		opt.OperationKey = o.key
		if until != "" {
			t, err := parseLimitTime("until", until)
			if err != nil {
				return err
			}
			opt.Until = &t
		}
		v, err := s.SetRouteLimit(c.Context(), selector, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	setCmd.Args = cobra.ExactArgs(1)
	setCmd.Flags().StringVar(&set.Kind, "kind", set.Kind, "Limit kind: quota_exhausted or rate_limited")
	setCmd.Flags().StringVar(&until, "until", "", "RFC3339 time when the limit ends")
	setCmd.Flags().DurationVar(&set.For, "for", 0, "Duration of the limit, such as 45m or 5h")
	setCmd.Flags().StringVar(&set.Scope, "scope", "", "Match scope: client, provider or route; defaults by client adapter")
	setCmd.Flags().StringVar(&set.Message, "message", "", "Short note shown with the record")
	group.AddCommand(setCmd)

	var clear core.RouteLimitClearOptions
	clearCmd := command("clear <target>", "Clear active usage limits that match a route", func(c *cobra.Command, args []string) error {
		s, selector, err := o.optionalScope()
		if err != nil {
			return err
		}
		opt := clear
		opt.Target = args[0]
		opt.OperationKey = o.key
		v, err := s.ClearRouteLimit(c.Context(), selector, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	clearCmd.Args = cobra.ExactArgs(1)
	clearCmd.Flags().StringVar(&clear.Scope, "scope", "", "Clear only records of this scope: client, provider or route")
	group.AddCommand(clearCmd)

	var report core.RouteLimitReportOptions
	var resetAt string
	usedPercent := -1
	reportCmd := command("report", "Report a usage limit for the calling Run's route", func(c *cobra.Command, _ []string) error {
		s, selector, err := o.optionalScope()
		if err != nil {
			return err
		}
		opt := report
		if resetAt != "" {
			t, err := parseLimitTime("reset-at", resetAt)
			if err != nil {
				return err
			}
			opt.ResetAt = &t
		}
		if usedPercent >= 0 {
			p := usedPercent
			opt.UsedPercent = &p
		}
		v, err := s.ReportRouteLimit(c.Context(), selector, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	reportCmd.Flags().StringVar(&report.Kind, "kind", "", "Limit kind: rate_limited, quota_exhausted or usage_pressure")
	reportCmd.Flags().StringVar(&resetAt, "reset-at", "", "RFC3339 reset time reported by the client")
	reportCmd.Flags().DurationVar(&report.RetryAfter, "retry-after", 0, "Duration until retry, such as 90s or 2h")
	reportCmd.Flags().IntVar(&usedPercent, "used-percent", -1, "Used share of the window, 0-100, for usage_pressure")
	reportCmd.Flags().StringVar(&report.Scope, "scope", "", "Match scope: client, provider or route; defaults by client adapter")
	reportCmd.Flags().StringVar(&report.Message, "message", "", "Short sanitized client message")
	_ = reportCmd.MarkFlagRequired("kind")
	group.AddCommand(reportCmd)
	return group
}

// optionalScope resolves the project and, when available, the workspace from
// --workspace, WORKSPACE_ID or the current directory.
func (o *options) optionalScope() (*core.Service, string, error) {
	scope, err := o.bootstrapScope(false)
	if err != nil {
		return nil, "", err
	}
	if !scope.ProjectFound {
		return nil, "", scope.ProjectError
	}
	return scope.Service, scope.WorkspaceID, nil
}

func parseLimitTime(flag, value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, &core.Error{Code: "invalid_argument", Message: "--" + flag + " must be an RFC3339 time"}
	}
	return t.UTC(), nil
}
