package cli

import (
	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func checkCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "check", Short: "Capture verification commands and evidence"}
	var opt core.CheckOptions
	run := command("run -- <command> [args...]", "Execute a check in the assigned worktree", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.Argv = args
		opt.OperationKey = o.key
		v, err := s.RunCheck(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	run.Args = cobra.MinimumNArgs(1)
	run.Flags().StringVar(&opt.Session, "session", "", "Producing session (inferred for agents)")
	group.AddCommand(run)
	group.AddCommand(command("list", "List captured check receipts", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		var receipts []core.CheckReceipt
		err = s.With(c.Context(), id, func(d *core.Document) error { receipts = d.Registry.Checks; return nil })
		if err != nil {
			return err
		}
		return o.emit(receipts)
	}))
	return group
}
