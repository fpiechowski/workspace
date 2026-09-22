package cli

import (
	"github.com/spf13/cobra"
)

func dispatcherCommands(o *options) (*cobra.Command, *cobra.Command) {
	group := &cobra.Command{Use: "dispatcher", Short: "Manage the project-scoped Dispatcher"}
	var profile string
	start := command("start", "Start or resume the project Dispatcher", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.StartDispatcher(c.Context(), profile, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	start.Flags().StringVar(&profile, "profile", "", "Override the configured Dispatcher model profile")
	group.AddCommand(start)
	group.AddCommand(command("status", "Read Dispatcher state and verified runtime ownership", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.DispatcherStatus(c.Context())
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	group.AddCommand(command("stop", "Stop the current Dispatcher Run and prevent automatic restart", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		if err := s.StopDispatcher(c.Context(), o.key); err != nil {
			return err
		}
		return o.emit(map[string]bool{"stopped": true})
	}))
	group.AddCommand(command("attach", "Attach to the verified project Dispatcher tmux session", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		return s.AttachDispatcher(c.Context())
	}))
	runner := command("_dispatcher-exec <run>", "Internal project Dispatcher runner", func(c *cobra.Command, args []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		return s.ExecuteDispatcher(c.Context(), args[0], o.in, o.out, o.errOut)
	})
	runner.Args = cobra.ExactArgs(1)
	runner.Hidden = true
	return group, runner
}
