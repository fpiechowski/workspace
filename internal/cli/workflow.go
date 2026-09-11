package cli

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"workspace/internal/core"
)

func integrationCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "integration", Short: "Prepare a shared checkout of accepted implementation tasks"}
	var opt core.IntegrationOptions
	prepare := command("prepare", "Create integration worktree and immutable input manifest", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.OperationKey = o.key
		v, err := s.PrepareIntegration(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	prepare.Flags().StringSliceVar(&opt.Tasks, "tasks", nil, "Accepted task IDs; defaults to all implementation tasks")
	prepare.Flags().StringVar(&opt.Base, "base", "", "Target base revision")
	group.AddCommand(prepare)
	return group
}
func decisionCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "decision", Short: "Record answers to workflow questions"}
	group.AddCommand(command("refresh", "Replace a stale question while preserving its history", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.RefreshDecision(c.Context(), id, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	var opt core.DecisionAnswer
	answer := command("answer <id>", "Record the user's answer to a specific decision revision", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.ID = args[0]
		v, err := s.AnswerDecision(c.Context(), id, opt, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	answer.Args = cobra.ExactArgs(1)
	f := answer.Flags()
	f.StringVar(&opt.Answer, "answer", "", "Selected option")
	f.StringVar(&opt.Reason, "reason", "", "User's explanation")
	f.StringVar(&opt.Environment, "environment", "", "Live-testing environment/URL")
	f.IntVar(&opt.ExpectedRevision, "expected-revision", 0, "Revision of the pending decision")
	f.BoolVar(&opt.UserConfirmed, "user-confirmed", false, "Orchestrator attests this answer was explicitly supplied by the user")
	group.AddCommand(answer)
	return group
}
func releaseCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "release", Short: "Record deployment/release confirmation"}
	var reference string
	var confirmed bool
	c := command("confirm", "Complete the workflow after explicit user release confirmation", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.ConfirmRelease(c.Context(), id, reference, confirmed, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	c.Flags().StringVar(&reference, "reference", "", "Release/deployment reference")
	c.Flags().BoolVar(&confirmed, "user-confirmed", false, "Orchestrator attests the user explicitly confirmed release")
	group.AddCommand(c)
	return group
}
func stateCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "state", Short: "Update durable state with revision checking"}
	var file string
	var expected int
	update := command("update", "Apply a typed YAML/JSON patch", func(c *cobra.Command, _ []string) error {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		patch, err := core.ParseStatePatch(b)
		if err != nil {
			return err
		}
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.UpdateState(c.Context(), id, expected, patch, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	update.Flags().StringVar(&file, "patch-file", "", "title, body, status, phase patch")
	update.Flags().IntVar(&expected, "expected-revision", 0, "Required current workspace revision")
	group.AddCommand(update)
	var editedFile string
	var editExpected int
	edit := command("edit", "Edit a paused workspace narrative/title via EDITOR or --file", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		var b []byte
		revision := editExpected
		if editedFile != "" {
			b, err = os.ReadFile(editedFile)
			if err != nil {
				return err
			}
		} else {
			if o.nonInteractive {
				return &core.Error{Code: "decision_required", Message: "non-interactive editing requires --file and --expected-revision", Options: []string{"provide-file", "open-editor-interactively"}}
			}
			b, revision, err = s.StateDocument(c.Context(), id)
			if err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				return &core.Error{Code: "editor_required", Message: "set EDITOR to an executable or use --file"}
			}
			f, err := os.CreateTemp("", "workspace-edit-*.md")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			if _, err := f.Write(b); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			cmd := exec.CommandContext(c.Context(), editor, f.Name())
			cmd.Stdin = o.in
			cmd.Stdout = o.out
			cmd.Stderr = o.errOut
			if err := cmd.Run(); err != nil {
				return err
			}
			b, err = os.ReadFile(f.Name())
			if err != nil {
				return err
			}
		}
		v, err := s.EditState(c.Context(), id, revision, b, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	edit.Flags().StringVar(&editedFile, "file", "", "Import edited WORKSPACE.md instead of opening EDITOR")
	edit.Flags().IntVar(&editExpected, "expected-revision", 0, "Required revision when using --file")
	group.AddCommand(edit)
	return group
}
