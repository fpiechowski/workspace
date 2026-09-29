package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// idReservation is the durable record for one allocated slug. Reservations are
// never released, so a slug is never reused even after its entity is deleted.
type idReservation struct {
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id,omitempty"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Operation   string    `json:"operation,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func reservationPath(storage, kind, slug string) string {
	return filepath.Join(storage, ".runtime", "ids", kind, slug)
}

// reservationOperation is the stable identity of a keyed create. A crashed
// create that retries with the same key reuses its own reservation instead of
// skipping to the next suffix. An empty key marks an unkeyed create.
func reservationOperation(projectID, workspaceID, kind, key string) string {
	if key == "" {
		return ""
	}
	ws := workspaceID
	if ws == "" {
		ws = "-"
	}
	return projectID + "/" + ws + "/" + kind + "/" + key
}

// idAllocator mints IDs of one kind inside one storage root. The reservation
// file is created with O_EXCL, which keeps allocation atomic across processes
// and across projects that share an external workspaces_dir.
type idAllocator struct {
	storage     string
	kind        string
	projectID   string
	workspaceID string
	operation   string
}

// reserve writes the reservation for slug. It returns created=true when this
// caller created it, otherwise it returns the record already on disk.
func (a idAllocator) reserve(prefix, slug string) (string, bool, *idReservation, error) {
	record := idReservation{
		Kind:        a.kind,
		ID:          prefix + "_" + slug,
		ProjectID:   a.projectID,
		WorkspaceID: a.workspaceID,
		Operation:   a.operation,
		CreatedAt:   nowUTC(),
	}
	path := reservationPath(a.storage, a.kind, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", false, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		b, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			f.Close()
			_ = os.Remove(path)
			return "", false, nil, marshalErr
		}
		if _, writeErr := f.Write(b); writeErr != nil {
			f.Close()
			_ = os.Remove(path)
			return "", false, nil, writeErr
		}
		if closeErr := f.Close(); closeErr != nil {
			return "", false, nil, closeErr
		}
		return record.ID, true, nil, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return "", false, nil, err
	}
	var existing idReservation
	if b, readErr := os.ReadFile(path); readErr == nil {
		_ = json.Unmarshal(b, &existing)
	}
	if existing.ID == "" {
		existing.ID = prefix + "_" + slug
	}
	return existing.ID, false, &existing, nil
}

// owns reports whether an existing reservation belongs to the same keyed
// operation, in which case the retry must keep the already allocated ID.
func (a idAllocator) owns(existing *idReservation) bool {
	return existing != nil && a.operation != "" && existing.Operation == a.operation
}

// allocate picks base, base-2, base-3, ... deterministically and reserves the
// first candidate that is neither in taken nor already reserved.
func (a idAllocator) allocate(prefix, base string, taken func(string) bool) (string, error) {
	for n := 1; n <= 1_000_000; n++ {
		candidate := base
		if n > 1 {
			suffix := "-" + strconv.Itoa(n)
			candidate = trimSlug(base, slugMaxLength-len(suffix)) + suffix
		}
		if !slugPattern.MatchString(candidate) || reservedSlug(candidate) {
			continue
		}
		if taken != nil && taken(candidate) {
			continue
		}
		id, created, existing, err := a.reserve(prefix, candidate)
		if err != nil {
			return "", err
		}
		if created || a.owns(existing) {
			return id, nil
		}
	}
	return "", fail("id_allocation_failed", "could not allocate a %s id for base %q", a.kind, base)
}

// allocateExplicit reserves an explicit slug. It never adds a suffix; a taken
// slug is refused with id_exists. A retry of the same keyed create reuses the
// reservation it made before crashing.
func (a idAllocator) allocateExplicit(prefix, slug string, taken func(string) bool) (string, error) {
	if taken != nil && taken(slug) {
		return "", fail("id_exists", "%s_%s already exists", prefix, slug)
	}
	id, created, existing, err := a.reserve(prefix, slug)
	if err != nil {
		return "", err
	}
	if created || a.owns(existing) {
		return id, nil
	}
	return "", fail("id_exists", "%s_%s already exists", prefix, slug)
}
