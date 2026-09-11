package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillInstallationPreservesExistingInstructions(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	path, err := s.InstallSkill(ctx, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(s.Root, ".agents", "skills", "workspace", "SKILL.md") {
		t.Fatal("not discoverable in project")
	}
	if _, err := s.InstallSkill(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.InstallSkill(ctx, "codex")
	expectCode(t, err, "skill_exists")
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "custom instructions" {
		t.Fatal("overwrote user skill")
	}
	if _, err := s.InstallSkill(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
}
