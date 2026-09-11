package cli

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"workspace/internal/core"
)

func taskCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "task", Short: "Manage delegated tasks, dependencies and attempts"}
	var file string
	create := command("create", "Create a task from YAML or Markdown frontmatter", func(c *cobra.Command, _ []string) error {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		spec, err := core.ParseTaskSpec(b)
		if err != nil {
			return err
		}
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.CreateTask(c.Context(), id, spec, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	create.Flags().StringVar(&file, "spec-file", "", "Task specification file")
	group.AddCommand(create)
	group.AddCommand(command("list", "List task states and dependencies", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Workspace.Tasks)
	}))
	var reason string
	retry := command("retry <task>", "Start a fresh attempt, invalidating dependent results", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.RetryTask(c.Context(), id, args[0], reason, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	retry.Args = cobra.ExactArgs(1)
	retry.Flags().StringVar(&reason, "reason", "Retry requested", "Reason for new attempt")
	group.AddCommand(retry)
	return group
}
func messageCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "message", Short: "Send durable messages to agent identities"}
	var opt core.MessageOptions
	var file string
	send := command("send", "Send a note, question or answer", func(c *cobra.Command, _ []string) error {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		opt.Body = string(b)
		opt.OperationKey = o.key
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.SendMessage(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	f := send.Flags()
	f.StringVar(&file, "body-file", "", "Message body file")
	f.StringVar(&opt.To, "to", "", "Recipient agent ID or name")
	f.StringVar(&opt.Kind, "kind", "note", "note, question, answer, status")
	f.StringVar(&opt.ReplyTo, "reply-to", "", "Original message ID")
	group.AddCommand(send)
	return group
}
func inboxCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "inbox", Short: "Read and acknowledge durable messages"}
	var recipient string
	var all bool
	list := command("list", "List pending messages", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Inbox(c.Context(), id, recipient, all)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	list.Flags().StringVar(&recipient, "agent", "", "Inbox owner (defaults to current agent/orchestrator)")
	list.Flags().BoolVar(&all, "all", false, "Include acknowledged messages")
	group.AddCommand(list)
	for _, verb := range []string{"read", "ack"} {
		verb := verb
		c := command(verb+" <message>", "Read a message; ack records receipt, not task acceptance", func(c *cobra.Command, args []string) error {
			s, id, err := o.scope()
			if err != nil {
				return err
			}
			v, err := s.ReadMessage(c.Context(), id, args[0], verb == "ack", o.key)
			if err != nil {
				return err
			}
			return o.emit(v)
		})
		c.Args = cobra.ExactArgs(1)
		group.AddCommand(c)
	}
	var timeout int
	var waitRecipient string
	wait := command("wait", "Wait for messages without holding a project lock", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.WaitInbox(c.Context(), id, waitRecipient, time.Duration(timeout)*time.Second)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	wait.Flags().IntVar(&timeout, "timeout", 30, "Timeout in seconds")
	wait.Flags().StringVar(&waitRecipient, "agent", "", "Inbox owner")
	group.AddCommand(wait)
	return group
}
func handoffCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "handoff", Short: "Preserve results and submit them for orchestrator review"}
	var opt core.HandoffOptions
	var summary, checks string
	submit := command("submit", "Copy artifacts and enqueue a result atomically", func(c *cobra.Command, _ []string) error {
		b, err := os.ReadFile(summary)
		if err != nil {
			return err
		}
		opt.Summary = string(b)
		opt.OperationKey = o.key
		if checks != "" {
			b, err := os.ReadFile(checks)
			if err != nil {
				return err
			}
			if err := yaml.Unmarshal(b, &opt.Checks); err != nil {
				return err
			}
		}
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.SubmitHandoff(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	f := submit.Flags()
	f.StringVar(&opt.To, "to", "", "Parent/orchestrator agent ID")
	f.StringVar(&opt.Task, "task", "", "Task ID/name")
	f.StringVar(&opt.Session, "session", "", "Producing session (inferred for agents)")
	f.StringVar(&opt.Outcome, "outcome", "succeeded", "succeeded, blocked, failed")
	f.StringVar(&summary, "summary-file", "", "Summary text file")
	f.StringSliceVar(&opt.Artifacts, "artifact", nil, "Explicit artifact path; repeat or comma-separate")
	f.StringVar(&checks, "checks-file", "", "YAML/JSON array of command, exit_code, evidence filename")
	f.StringSliceVar(&opt.CheckIDs, "check", nil, "Captured check receipt ID; repeat for multiple checks")
	f.StringSliceVar(&opt.Risks, "risk", nil, "Known risks")
	group.AddCommand(submit)
	group.AddCommand(command("list", "List result submissions", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		var result []core.Handoff
		err = s.With(c.Context(), id, func(d *core.Document) error { result = d.Registry.Handoffs; return nil })
		if err != nil {
			return err
		}
		return o.emit(result)
	}))
	for _, verb := range []string{"accept", "reject"} {
		verb := verb
		var file string
		c := command(verb+" <handoff>", "Review a handoff separately from inbox acknowledgement", func(c *cobra.Command, args []string) error {
			feedback := ""
			if file != "" {
				b, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				feedback = string(b)
			}
			s, id, err := o.scope()
			if err != nil {
				return err
			}
			v, err := s.ReviewHandoff(c.Context(), id, args[0], verb == "accept", feedback, o.key)
			if err != nil {
				return err
			}
			return o.emit(v)
		})
		c.Args = cobra.ExactArgs(1)
		c.Flags().StringVar(&file, "reason-file", "", "Review feedback (required to reject)")
		group.AddCommand(c)
	}
	return group
}
func artifactCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "artifact", Short: "Inspect preserved work products"}
	group.AddCommand(command("list", "List artifacts and producing sessions", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Workspace.Artifacts)
	}))
	return group
}
