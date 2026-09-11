package core

import "path/filepath"

func (s *Service) storageRoot(cfg Config) (string, error) {
	if cfg.WorkspacesDir == "" {
		return filepath.Join(s.Root, ".workspace"), nil
	}
	root := cfg.WorkspacesDir
	if !filepath.IsAbs(root) {
		root = filepath.Join(s.Root, root)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if root == filepath.Clean(s.Root) || contained(filepath.Join(s.Root, ".git"), root) {
		return "", fail("invalid_config", "workspaces_dir cannot be the repository root or Git metadata")
	}
	return root, nil
}
