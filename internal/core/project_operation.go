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

// Project effects (initialization and skill installation) already preserve
// existing files. A separate receipt prevents replay from repeating later work.
func projectEffect[T any](ctx context.Context, root, key string, request any, fn func() (T, error)) (T, error) {
	var out T
	path := filepath.Join(root, ".workspace", ".runtime", "operations", strings.TrimPrefix(digest([]byte(key)), "sha256:")+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return out, err
	}
	l := flock.New(path + ".lock")
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	locked, err := l.TryLockContext(lockCtx, 25*time.Millisecond)
	if err != nil {
		return out, err
	}
	if !locked {
		return out, fail("operation_busy", "project operation is already executing")
	}
	defer l.Unlock()
	r := MutationReceipt{ID: ID("op"), Digest: payloadDigest(request), State: "pending"}
	var previous MutationReceipt
	if err := readJSON(path, &previous); err == nil {
		if previous.Digest != r.Digest {
			return out, fail("operation_conflict", "operation key %q was already used with another payload", key)
		}
		r = previous
		if r.State == "completed" {
			err := json.Unmarshal(r.Result, &out)
			return out, err
		}
	} else if !os.IsNotExist(err) {
		return out, err
	}
	if err := writeJSON(path, r); err != nil {
		return out, err
	}
	out, err = fn()
	if err != nil {
		return out, err
	}
	r.Result, err = json.Marshal(out)
	if err != nil {
		return out, err
	}
	r.State = "completed"
	return out, writeJSON(path, r)
}
