package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// MutationReceipt is committed with the state change, so a lost CLI response can
// be replayed even after the workspace has advanced to a different revision.
type MutationReceipt struct {
	ID       string          `json:"id"`
	Digest   string          `json:"digest"`
	Revision int             `json:"revision"`
	Result   json.RawMessage `json:"result"`
	State    string          `json:"state"`
}

func mutationKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// mutate is for operations whose durable effects are confined to Document and
// PendingFiles. External processes must retain their own intent/recovery protocol.
func mutate[T any](s *Service, ctx context.Context, selector string, keys []string,
	request any, out *T, authorize func(*Document) error, fn func(*Document) error) error {
	return s.With(ctx, selector, func(d *Document) error {
		if err := authorize(d); err != nil {
			return err
		}
		key := mutationKey(keys)
		if key == "" {
			return fn(d)
		}
		hash := payloadDigest(request)
		if receipt, ok := d.Registry.Mutations[key]; ok {
			if receipt.Digest != hash {
				return fail("operation_conflict", "operation key %q was already used with another payload", key)
			}
			if receipt.State != "completed" {
				return fail("operation_pending", "operation has an unfinished external effect")
			}
			return json.Unmarshal(receipt.Result, out)
		}
		if _, ok := d.Registry.Operations[key]; ok {
			return fail("operation_conflict", "operation key %q belongs to another operation", key)
		}
		d.deferSave = true
		if err := fn(d); err != nil {
			return err
		}
		d.deferSave = false
		result, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if d.Registry.Mutations == nil {
			d.Registry.Mutations = map[string]MutationReceipt{}
		}
		d.Registry.Mutations[key] = MutationReceipt{ID: ID("op"), Digest: hash, Revision: d.State.Revision, Result: result, State: "completed"}
		return flushDocument(d)
	})
}

// effect serializes retries of a recoverable external operation without holding
// the project lock during network/process work. A pending receipt is deliberately
// retried through the operation's existing reconciliation protocol after a crash.
func effect[T any](s *Service, ctx context.Context, selector, key string, request any,
	authorize func(*Document) error, fn func() (T, error)) (T, error) {
	var out T
	dir, err := s.resolve(selector)
	if err != nil {
		return out, err
	}
	path := filepath.Join(dir, ".runtime", "operation-locks", strings.TrimPrefix(digest([]byte(key)), "sha256:")+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return out, err
	}
	l := flock.New(path)
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	locked, err := l.TryLockContext(lockCtx, 25*time.Millisecond)
	if err != nil {
		return out, err
	}
	if !locked {
		return out, fail("operation_busy", "operation is already executing")
	}
	defer l.Unlock()
	hash := payloadDigest(request)
	replay := false
	err = s.With(ctx, selector, func(d *Document) error {
		if err := authorize(d); err != nil {
			return err
		}
		if receipt, ok := d.Registry.Mutations[key]; ok {
			if receipt.Digest != hash {
				return fail("operation_conflict", "operation key %q was already used with another payload", key)
			}
			if receipt.State == "completed" {
				replay = true
				return json.Unmarshal(receipt.Result, &out)
			}
			return nil
		}
		if _, ok := d.Registry.Operations[key]; ok {
			return fail("operation_conflict", "operation key %q belongs to another operation", key)
		}
		if d.Registry.Mutations == nil {
			d.Registry.Mutations = map[string]MutationReceipt{}
		}
		d.Registry.Mutations[key] = MutationReceipt{ID: ID("op"), Digest: hash, State: "pending"}
		return flushDocument(d)
	})
	if err != nil || replay {
		return out, err
	}
	out, err = fn()
	if err != nil {
		return out, err
	}
	result, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	err = s.With(context.Background(), selector, func(d *Document) error {
		r := d.Registry.Mutations[key]
		r.State, r.Result, r.Revision = "completed", result, d.State.Revision
		d.Registry.Mutations[key] = r
		return flushDocument(d)
	})
	return out, err
}
