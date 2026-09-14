package core

import (
	"bytes"
	"context"
	"embed"
	"os"
	"path/filepath"
)

//go:embed skill/workspace/SKILL.md
var bundledSkill embed.FS

func (s *Service) InstallSkill(ctx context.Context, client string, keys ...string) (string, error) {
	if s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "" {
		return "", fail("forbidden", "install the project skill from a user terminal")
	}
	if key := mutationKey(keys); key != "" {
		return projectEffect(ctx, s.Root, key, []any{"skill.install", client}, func() (string, error) { return s.InstallSkill(ctx, client) })
	}
	root := ".agents"
	switch client {
	case "", "codex", "opencode":
	case "claude":
		root = ".claude"
	default:
		return "", fail("invalid_client", "skill clients: codex, claude, opencode")
	}
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return "", err
	}
	defer unlock()
	path := filepath.Join(s.Root, root, "skills", "workspace", "SKILL.md")
	b, err := bundledSkill.ReadFile("skill/workspace/SKILL.md")
	if err != nil {
		return "", err
	}
	if old, err := os.ReadFile(path); err == nil {
		if bytes.Equal(old, b) {
			return path, nil
		}
		return "", fail("skill_exists", "existing skill differs; inspect %s before replacing it", path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := atomicWrite(path, b); err != nil {
		return "", err
	}
	return path, nil
}
