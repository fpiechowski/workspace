package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type PreviewKind string

const (
	PreviewDocument PreviewKind = "document"
	PreviewArtifact PreviewKind = "artifact"
	PreviewCheck    PreviewKind = "check"
	MaxPreviewBytes             = 256 * 1024
)

type Preview struct {
	Kind       PreviewKind `json:"kind"`
	ResourceID string      `json:"resource_id"`
	Name       string      `json:"name,omitempty"`
	Path       string      `json:"path,omitempty"`
	Digest     string      `json:"digest,omitempty"`
	Size       int64       `json:"size"`
	Text       string      `json:"text,omitempty"`
	Truncated  bool        `json:"truncated"`
	Binary     bool        `json:"binary"`
	Warning    string      `json:"warning,omitempty"`
}

// ReadPreview resolves a closed resource kind and ID through the workspace
// registry. Callers cannot provide an arbitrary filesystem path.
func (s *Service) ReadPreview(ctx context.Context, workspaceID string, kind PreviewKind, resourceID string) (Preview, error) {
	var out Preview
	err := s.With(ctx, workspaceID, func(d *Document) error {
		var path, root, name, expectedDigest string
		out = Preview{Kind: kind, ResourceID: resourceID}
		switch kind {
		case PreviewDocument:
			root = d.Dir
			switch resourceID {
			case "workspace", "WORKSPACE.md":
				path = filepath.Join(d.Dir, "WORKSPACE.md")
				name = "WORKSPACE.md"
			case "workflow", "WORKFLOW.md":
				path = filepath.Join(d.Dir, "WORKFLOW.md")
				name = "WORKFLOW.md"
			case "input", "input-snapshot":
				if filepath.IsAbs(d.State.Input.Snapshot) || !contained(d.Dir, filepath.Join(d.Dir, d.State.Input.Snapshot)) {
					return fail("unsafe_path", "input snapshot is outside the workspace")
				}
				path = filepath.Join(d.Dir, d.State.Input.Snapshot)
				name = filepath.Base(path)
			default:
				return fail("preview_not_found", "unknown workspace document %q", resourceID)
			}
		case PreviewArtifact:
			artifact, err := findArtifact(d, resourceID)
			if err != nil {
				return err
			}
			root = filepath.Join(d.Dir, "artifacts")
			path = filepath.Join(d.Dir, artifact.Path)
			name = artifact.Name
			expectedDigest = artifact.Digest
			out.Digest = artifact.Digest
		case PreviewCheck:
			check, err := findCheck(d, resourceID)
			if err != nil {
				return err
			}
			root = filepath.Join(d.Dir, ".runtime", "checks")
			path = filepath.Join(root, check.ID+".log")
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				// Older workspaces kept only the task-worktree copy. Resolve it
				// through the stored Session and Worktree lineage, never Output.
				session, sessionErr := findSession(d, check.SessionID)
				if sessionErr != nil {
					return fail("preview_not_found", "captured check output is unavailable")
				}
				worktree, worktreeErr := findWorktree(d, session.WorktreeID)
				if worktreeErr != nil {
					return fail("preview_not_found", "captured check output is unavailable")
				}
				root = filepath.Join(worktree.Path, "work-products", "checks")
				path = filepath.Join(root, check.ID+".log")
			}
			name = check.ID + ".log"
			out.Digest = check.Digest
		default:
			return fail("invalid_preview_kind", "unsupported preview kind %q", kind)
		}
		out.Name = name
		preview, err := readPreviewFile(path, root)
		if err != nil {
			return err
		}
		out.Size, out.Text, out.Truncated, out.Binary = preview.size, preview.text, preview.truncated, preview.binary
		if preview.truncated {
			out.Warning = "preview truncated at 256 KiB"
		}
		if !out.Binary && !out.Truncated && expectedDigest != "" && digest([]byte(out.Text)) != expectedDigest {
			out.Warning = "artifact digest does not match the recorded digest"
		}
		return nil
	})
	return out, err
}

type previewRead struct {
	size      int64
	text      string
	truncated bool
	binary    bool
}

func readPreviewFile(path, root string) (previewRead, error) {
	var out previewRead
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, fail("preview_not_found", "preview file is unavailable")
		}
		return out, err
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, fail("preview_not_found", "preview file is unavailable")
		}
		return out, err
	}
	if !contained(canonicalRoot, canonicalPath) {
		return out, fail("unsafe_path", "preview path resolves outside its allowed directory")
	}
	info, err := os.Stat(canonicalPath)
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() {
		return out, fail("unsafe_path", "preview source is not a regular file")
	}
	out.size = info.Size()
	f, err := os.Open(canonicalPath)
	if err != nil {
		return out, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxPreviewBytes+1))
	if err != nil {
		return out, err
	}
	if len(b) > MaxPreviewBytes {
		out.truncated = true
		b = b[:MaxPreviewBytes]
	}
	out.binary = strings.IndexByte(string(b), 0) >= 0 || !utf8.Valid(b)
	if !out.binary {
		out.text = string(b)
	}
	return out, nil
}
