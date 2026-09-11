package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func supervisorCommands(o *options) (*cobra.Command, *cobra.Command) {
	serve := command("serve", "Supervise project sessions and durable message delivery", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		if s.Actor.AgentID != "" {
			return &core.Error{Code: "role_forbidden", Message: "supervisor must run outside an agent session"}
		}
		ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return s.Serve(ctx)
	})
	group := &cobra.Command{Use: "server", Short: "Inspect or stop the project supervisor"}
	group.AddCommand(command("status", "Read supervisor status", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.SupervisorStatus(c.Context())
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	group.AddCommand(command("stop", "Stop supervision; agent processes remain running", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		if s.Actor.AgentID != "" {
			return &core.Error{Code: "role_forbidden", Message: "stop the supervisor from a user terminal"}
		}
		if err := s.StopSupervisor(c.Context(), o.key); err != nil {
			return err
		}
		return o.emit(map[string]bool{"stopping": true})
	}))
	return serve, group
}
