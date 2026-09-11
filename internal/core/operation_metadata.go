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

// OperationMetadata reports the revision at which the operation was recorded,
// rather than claiming that its historical result describes the latest state.
func (s *Service) OperationMetadata(ctx context.Context, selector, key string) (OperationMetadata, error) {
	var out OperationMetadata
	if selector == "" {
		path := filepath.Join(s.Root, ".workspace", ".runtime", "operations", strings.TrimPrefix(digest([]byte(key)), "sha256:")) + ".json"
		var r MutationReceipt
		if err := readJSON(path, &r); err != nil {
			if os.IsNotExist(err) {
				return out, nil
			}
			return out, err
		}
		return OperationMetadata{ID: r.ID, Revision: r.Revision}, nil
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
