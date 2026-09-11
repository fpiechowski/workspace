package cli

import (
	"os"

	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func changeRequestCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "change-request", Short: "Prepare, publish and reconcile change requests"}
	var opt core.ChangeRequestOptions
	var bodyFile string
	prepare := command("prepare", "Prepare a title, description and diff before publishing", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		if bodyFile != "" {
			b, err := os.ReadFile(bodyFile)
			if err != nil {
				return err
			}
			opt.Body = string(b)
		}
		opt.OperationKey = o.key
		v, err := s.PrepareChangeRequest(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	f := prepare.Flags()
	f.StringVar(&opt.Worktree, "worktree", "", "Worktree (defaults to integration)")
	f.StringVar(&opt.Target, "target", "", "Target branch")
	f.StringVar(&opt.Title, "title", "", "Change-request title")
	f.StringVar(&bodyFile, "body-file", "", "Change-request description")
	group.AddCommand(prepare)
	var confirmed bool
	publish := command("publish <id>", "Publish once; reconcile uncertain outcomes before retry", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.PublishChangeRequest(c.Context(), id, args[0], confirmed, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	publish.Args = cobra.ExactArgs(1)
	publish.Flags().BoolVar(&confirmed, "user-confirmed", false, "Orchestrator attests the user approved the prepared diff and description")
	group.AddCommand(publish)
	group.AddCommand(command("sync", "Refresh external change-request state without creating requests", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.SyncChangeRequests(c.Context(), id, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	group.AddCommand(command("list", "List prepared and published change requests", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Workspace.ChangeRequests)
	}))
	for _, verb := range []string{"skip", "link", "retry"} {
		verb := verb
		var reason, reference string
		var userConfirmed bool
		c := command(verb+" <id>", "Record the user's publication decision", func(c *cobra.Command, args []string) error {
			s, id, err := o.scope()
			if err != nil {
				return err
			}
			v, err := s.ResolveChangeRequest(c.Context(), id, args[0], verb, reference, reason, userConfirmed, o.key)
			if err != nil {
				return err
			}
			return o.emit(v)
		})
		c.Args = cobra.ExactArgs(1)
		c.Flags().StringVar(&reason, "reason", "", "User's reason/evidence")
		c.Flags().StringVar(&reference, "url", "", "Existing change-request URL")
		c.Flags().BoolVar(&userConfirmed, "user-confirmed", false, "Orchestrator attests explicit user decision")
		group.AddCommand(c)
	}
	return group
}
