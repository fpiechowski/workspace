package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"gopkg.in/yaml.v3"
)

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
func payloadDigest(value any) string { b, _ := json.Marshal(value); return digest(b) }
func strictYAML(b []byte, v any) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fail("invalid_yaml", "expected one YAML document")
	}
	return nil
}

// Keep the temporary file on the destination filesystem; readers see one version.
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	// Windows does not support opening directories for File.Sync. Session runtime
	// durability targets Unix filesystems; the portable core still runs on Windows.
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(b, '\n'))
}
func readJSON(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

var safeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

func validateName(name string) error {
	if !safeName.MatchString(name) {
		return fail("invalid_name", "use 1–48 lowercase letters, digits or hyphens, starting with a letter")
	}
	return nil
}
func contained(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func lockProject(ctx context.Context, root string) (func(), error) {
	p := filepath.Join(root, ".workspace", ".runtime", "project.lock")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, err
	}
	l := flock.New(p)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ok, err := l.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil || !ok {
		return nil, fail("project_busy", "cannot acquire project lock: %v", err)
	}
	return func() { _ = l.Unlock() }, nil
}

func encodeDocument(d *Document) ([]byte, error) {
	b, err := yaml.Marshal(d.State)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(b) + "---\n" + d.Body), nil
}
func decodeDocument(b []byte, d *Document) error {
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return fail("invalid_state", "WORKSPACE.md has no YAML frontmatter")
	}
	head, body, ok := strings.Cut(text[4:], "\n---\n")
	if !ok {
		return fail("invalid_state", "unclosed YAML frontmatter")
	}
	if err := strictYAML([]byte(head), &d.State); err != nil {
		return fail("invalid_state", "%v", err)
	}
	if d.State.SchemaVersion != 1 || d.State.ID == "" || d.State.Revision < 1 {
		return fail("invalid_state", "unsupported schema or missing identity/revision")
	}
	d.Body = body
	return nil
}

// A write-ahead record makes the document + runtime index recoverable as a pair.
// Only the project lock holder may recover or modify these files.
type pendingWrite struct {
	Files        map[string][]byte `json:"files,omitempty"`
	BeforeDigest string            `json:"before_digest"`
	Document     []byte            `json:"document"`
	Registry     Registry          `json:"registry"`
}

func recoverWrite(dir string) error {
	p := filepath.Join(dir, ".runtime", "pending.json")
	var pending pendingWrite
	if err := readJSON(p, &pending); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	current, err := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
	if err != nil {
		return err
	}
	if digest(current) != pending.BeforeDigest && digest(current) != digest(pending.Document) {
		return fail("revision_conflict", "WORKSPACE.md changed during an interrupted write; preserve it before recovery")
	}
	for name, b := range pending.Files {
		path := filepath.Join(dir, name)
		if filepath.IsAbs(name) || !contained(dir, path) {
			return fail("unsafe_path", "invalid pending file %s", name)
		}
		if err := atomicWrite(path, b); err != nil {
			return err
		}
	}
	if err := atomicWrite(filepath.Join(dir, "WORKSPACE.md"), pending.Document); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, ".runtime", "index.json"), pending.Registry); err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(p))
}
func loadDocument(dir string) (*Document, error) {
	if err := recoverWrite(dir); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
	if err != nil {
		return nil, err
	}
	d := &Document{Dir: dir}
	if err := decodeDocument(b, d); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, ".runtime", "index.json"), &d.Registry); err != nil {
		return nil, err
	}
	if d.Registry.WorkspaceDigest != digest(b) {
		return nil, fail("revision_conflict", "WORKSPACE.md was edited outside workspace")
	}
	if d.Registry.Operations == nil {
		d.Registry.Operations = map[string]Operation{}
	}
	if migrateRegistry(d) || repairPersistedRuntime(d) {
		if err := saveDocument(d); err != nil {
			return nil, err
		}
	}
	return d, nil
}
func saveDocument(d *Document) error {
	d.Registry.SchemaVersion = registrySchemaVersion
	d.syncSessions()
	d.State.Revision++
	if d.deferSave {
		return nil
	}
	return flushDocument(d)
}

func flushDocument(d *Document) error {
	before := d.Registry.WorkspaceDigest
	for key, op := range d.Registry.Operations {
		if op.Revision == 0 {
			op.Revision = d.State.Revision
			d.Registry.Operations[key] = op
		}
	}
	b, err := encodeDocument(d)
	if err != nil {
		return err
	}
	d.Registry.WorkspaceDigest = digest(b)
	p := pendingWrite{BeforeDigest: before, Document: b, Registry: d.Registry, Files: d.PendingFiles}
	if err := writeJSON(filepath.Join(d.Dir, ".runtime", "pending.json"), p); err != nil {
		return err
	}
	if err := recoverWrite(d.Dir); err != nil {
		return err
	}
	d.PendingFiles = nil
	return nil
}
func (d *Document) previous(key string, request any) (string, error) {
	if key == "" {
		return "", nil
	}
	if _, exists := d.Registry.Mutations[key]; exists {
		return "", fail("operation_conflict", "operation key %q belongs to another operation", key)
	}
	op, exists := d.Registry.Operations[key]
	if !exists {
		return "", nil
	}
	if op.Digest != payloadDigest(request) {
		return "", fail("operation_conflict", "operation key %q was already used with another payload", key)
	}
	return op.ResourceID, nil
}
func (d *Document) remember(key string, request any, id string) {
	if key != "" {
		d.Registry.Operations[key] = Operation{ID: ID("op"), Digest: payloadDigest(request), ResourceID: id}
	}
}

func replayResource[T any](d *Document, key string, out *T) (bool, error) {
	op := d.Registry.Operations[key]
	if len(op.Result) == 0 {
		return false, nil
	}
	return true, json.Unmarshal(op.Result, out)
}

// Commit the resource response with its final state. Pending external intents
// keep Result empty until they have a concrete result to report.
func saveResource(d *Document, key string, out any) error {
	if key != "" {
		op, ok := d.Registry.Operations[key]
		if !ok {
			return fail("operation_missing", "resource operation has no durable intent")
		}
		b, err := json.Marshal(out)
		if err != nil {
			return err
		}
		op.Result = b
		op.Revision = d.State.Revision + 1
		d.Registry.Operations[key] = op
	}
	return saveDocument(d)
}
