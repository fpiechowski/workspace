package cli

import (
	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func serviceCommands(o *options) (*cobra.Command, *cobra.Command) {
	group := &cobra.Command{Use: "service", Short: "Run auxiliary processes in worktree panes"}
	var opt core.ServiceOptions
	start := command("start <name> -- <command> [args...]", "Start a worktree service", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.Name = args[0]
		opt.Argv = args[1:]
		opt.OperationKey = o.key
		if err := s.EnsureSupervisor(c.Context()); err != nil {
			return err
		}
		v, err := s.StartService(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	start.Args = cobra.MinimumNArgs(2)
	start.Flags().StringVar(&opt.Worktree, "worktree", "", "Assigned worktree")
	group.AddCommand(start)
	group.AddCommand(command("list", "List service history", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		var v []core.BackgroundService
		err = s.With(c.Context(), id, func(d *core.Document) error { v = d.Registry.Services; return nil })
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	stop := command("stop <id>", "Stop an owned service pane", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.StopService(c.Context(), id, args[0], o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	stop.Args = cobra.ExactArgs(1)
	group.AddCommand(stop)
	runner := command("_service-exec <id>", "Internal service runner", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		s.Actor = core.Actor{}
		return s.ExecuteService(c.Context(), id, args[0], o.in, o.out, o.errOut)
	})
	runner.Args = cobra.ExactArgs(1)
	runner.Hidden = true
	return group, runner
}
