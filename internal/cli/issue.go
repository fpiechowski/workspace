package cli

import (
	"os"

	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func issueCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "issue", Short: "Manage durable project Issues and linked Workspaces"}
	var create core.IssueCreateOptions
	var inputFile string
	createCmd := command("create [intent]", "Create or revise a project Issue", func(c *cobra.Command, args []string) error {
		if len(args) > 1 {
			return &core.Error{Code: "input_conflict", Message: "provide one Issue intent"}
		}
		if len(args) == 1 && inputFile != "" {
			return &core.Error{Code: "input_conflict", Message: "provide the Issue intent either as an argument or with --input-file, not both"}
		}
		if len(args) == 1 {
			create.Body = args[0]
		}
		if inputFile != "" {
			b, err := os.ReadFile(inputFile)
			if err != nil {
				return err
			}
			create.Body = string(b)
		}
		create.OperationKey = o.key
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.IntakeIssue(c.Context(), create)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	createCmd.Args = cobra.MaximumNArgs(1)
	createCmd.Flags().StringVar(&create.Title, "title", "", "Issue title")
	createCmd.Flags().StringVar(&inputFile, "input-file", "", "File containing the Issue description")
	createCmd.Flags().StringVar(&create.Source, "issue", "", "HTTP(S) source URL; fetch only when no local body is supplied")
	group.AddCommand(createCmd)

	group.AddCommand(command("list", "List project Issue summaries", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.ListIssues(c.Context())
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	show := command("show <issue>", "Show one Issue and its derived Workspace links", func(c *cobra.Command, args []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.ShowIssue(c.Context(), args[0])
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	show.Args = cobra.ExactArgs(1)
	group.AddCommand(show)
	var expected int
	refresh := command("refresh <issue>", "Refresh a sourced Issue from the configured read-only tracker", func(c *cobra.Command, args []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.RefreshIssue(c.Context(), args[0], core.IssueRefreshOptions{ExpectedRevision: expected, OperationKey: o.key})
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	refresh.Args = cobra.ExactArgs(1)
	refresh.Flags().IntVar(&expected, "expected-revision", 0, "Required current Issue revision")
	group.AddCommand(refresh)
	var status, reason string
	var updateExpected int
	update := command("update <issue>", "Change an Issue's local status", func(c *cobra.Command, args []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.UpdateIssue(c.Context(), args[0], core.IssueUpdateOptions{Status: status, Reason: reason, ExpectedRevision: updateExpected, OperationKey: o.key})
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	update.Args = cobra.ExactArgs(1)
	update.Flags().StringVar(&status, "status", "", "Local status: open, deferred, or closed")
	update.Flags().StringVar(&reason, "reason", "", "Reason for deferring, closing, or reopening")
	update.Flags().IntVar(&updateExpected, "expected-revision", 0, "Required current Issue revision")
	group.AddCommand(update)

	var dispatch core.IssueDispatchOptions
	dispatchCmd := command("dispatch <issue>", "Create a linked Workspace from an Issue and optionally start its Orchestrator", func(c *cobra.Command, args []string) error {
		dispatch.IssueID = args[0]
		dispatch.OperationKey = o.key
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.DispatchIssue(c.Context(), dispatch)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	dispatchCmd.Args = cobra.ExactArgs(1)
	dispatchCmd.Flags().StringVar(&dispatch.Workflow, "workflow", "", "Configured workflow; omit to choose later")
	dispatchCmd.Flags().BoolVar(&dispatch.NoWorkflow, "no-workflow", false, "Create an active manual Workspace")
	dispatchCmd.Flags().StringVar(&dispatch.Base, "base", "HEAD", "Base Git revision")
	dispatchCmd.Flags().StringVar(&dispatch.Title, "title", "", "Workspace title override")
	dispatchCmd.Flags().BoolVar(&dispatch.Start, "start", false, "Also start the linked Workspace Orchestrator")
	group.AddCommand(dispatchCmd)
	return group
}
