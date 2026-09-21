package core

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed skill/workspace/SKILL.md
var bundledSkill embed.FS

const bundledSkillPath = "skill/workspace/SKILL.md"

func readBundledSkill() ([]byte, error) {
	return bundledSkill.ReadFile(bundledSkillPath)
}

func stripSkillFrontMatter(b []byte) (string, error) {
	const opening = "---\n"
	const closing = "\n---\n"
	if !bytes.HasPrefix(b, []byte(opening)) {
		return "", fmt.Errorf("bundled workspace skill is missing leading YAML front matter")
	}

	rest := b[len(opening):]
	end := bytes.Index(rest, []byte(closing))
	if end < 0 {
		return "", fmt.Errorf("bundled workspace skill has malformed YAML front matter")
	}

	body := rest[end+len(closing):]
	if len(bytes.TrimSpace(body)) == 0 {
		return "", fmt.Errorf("bundled workspace skill has an empty Markdown body")
	}
	return string(body), nil
}

// PrimeInstructions returns the current binary's bundled general workspace
// guidance without its YAML front matter. It does not inspect project state.
func PrimeInstructions() (string, error) {
	b, err := readBundledSkill()
	if err != nil {
		return "", err
	}
	return stripSkillFrontMatter(b)
}

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
	b, err := readBundledSkill()
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
