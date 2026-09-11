package cli

import (
	"github.com/spf13/cobra"
	"os"
	"workspace/internal/core"
)

func revisionCommand(o *options, migrate bool) *cobra.Command {
	var opt core.RevisionOptions
	var file string
	use, short := "update", "Replace input, preserve history and invalidate affected results"
	if migrate {
		use, short = "migrate", "Apply updated templates with history and result invalidation"
	}
	c := command(use, short, func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.OperationKey = o.key
		var v core.Status
		if migrate {
			v, err = s.MigrateWorkflow(c.Context(), id, opt)
		} else {
			b, readErr := os.ReadFile(file)
			if readErr != nil {
				return readErr
			}
			opt.Input = string(b)
			v, err = s.UpdateInput(c.Context(), id, opt)
		}
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	c.Flags().StringVar(&opt.Reason, "reason", "", "Why this revision is required")
	c.Flags().IntVar(&opt.ExpectedRevision, "expected-revision", 0, "Current workspace revision")
	if !migrate {
		c.Flags().StringVar(&file, "input-file", "", "Revised issue description")
		c.Flags().StringVar(&opt.Source, "issue", "", "Source URL for revised input")
	}
	return c
}
