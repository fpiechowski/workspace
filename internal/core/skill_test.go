package core

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestPrimeInstructionsMatchesBundledSkillBody(t *testing.T) {
	full, err := readBundledSkill()
	if err != nil {
		t.Fatal(err)
	}
	const closing = "\n---\n"
	end := bytes.Index(full, []byte(closing))
	if end < 0 {
		t.Fatal("bundled skill has no front matter boundary")
	}
	want := string(full[end+len(closing):])

	got, err := PrimeInstructions()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatal("prime instructions differ from the bundled Markdown body")
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatal("prime instructions do not preserve the final newline")
	}
	if strings.HasPrefix(strings.TrimSpace(got), "---") || !strings.Contains(got, "# Workspace") {
		t.Fatal("prime instructions are not the expected Markdown body")
	}

	s, _ := fixture(t)
	path, err := s.InstallSkill(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, full) {
		t.Fatal("skill installation did not use the same embedded bytes")
	}
}

func TestStripSkillFrontMatterRejectsMalformedInput(t *testing.T) {
	for _, input := range [][]byte{
		[]byte("# Workspace\n"),
		[]byte("---\nname: workspace\n"),
		[]byte("---\nname: workspace\n---\n"),
	} {
		if _, err := stripSkillFrontMatter(input); err == nil {
			t.Fatalf("accepted malformed skill: %q", input)
		}
	}
}
