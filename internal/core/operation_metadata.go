package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type OperationMetadata struct {
	ID       string `json:"operation_id"`
	Revision int    `json:"revision,omitempty"`
}

func loadDispatcherStateForMetadata(root string) (dispatcherState, bool, error) {
	s := &Service{Root: root}
	cfg, err := s.Config()
	if err != nil {
		return dispatcherState{}, false, err
	}
	return loadDispatcherState(root, cfg.ProjectID)
}

// OperationMetadata reports the revision at which the operation was recorded,
// rather than claiming that its historical result describes the latest state.
func (s *Service) OperationMetadata(ctx context.Context, selector, key string) (OperationMetadata, error) {
	var out OperationMetadata
	if selector == "" {
		path := filepath.Join(s.Root, ".workspace", ".runtime", "operations", strings.TrimPrefix(digest([]byte(key)), "sha256:")) + ".json"
		var r MutationReceipt
		if err := readJSON(path, &r); err == nil {
			return OperationMetadata{ID: r.ID, Revision: r.Revision}, nil
		} else if !os.IsNotExist(err) {
			return out, err
		}
		// Project Issues keep their receipts beside the durable Issue store so a
		// partial Issue write and its operation can be recovered together.
		var issueReceipt issueReceipt
		if err := readJSON(issueOperationPath(s.Root, key), &issueReceipt); err == nil {
			return OperationMetadata{ID: issueReceipt.ID, Revision: issueReceipt.Revision}, nil
		} else if !os.IsNotExist(err) {
			return out, err
		}
		// Dispatcher start receipts live in the project-scoped state document.
		if state, exists, err := loadDispatcherStateForMetadata(s.Root); err != nil {
			return out, err
		} else if exists {
			if receipt, ok := state.Receipts[dispatcherReceiptKey(key)]; ok {
				return OperationMetadata{ID: receipt.ID, Revision: receipt.Revision}, nil
			}
			if receipt, ok := state.Receipts[dispatcherStopReceiptKey(key)]; ok {
				return OperationMetadata{ID: receipt.ID, Revision: receipt.Revision}, nil
			}
		}
		return out, nil
	}
	err := s.With(ctx, selector, func(d *Document) error {
		if r, ok := d.Registry.Mutations[key]; ok {
			out = OperationMetadata{ID: r.ID, Revision: r.Revision}
		} else {
			r := d.Registry.Operations[key]
			out = OperationMetadata{ID: r.ID, Revision: r.Revision}
		}
		return nil
	})
	return out, err
}
