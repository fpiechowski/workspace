package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"workspace/internal/terminal"
)

// ClientPreferenceStore holds the local UI-only last-used tmux client.
// Implementations are scoped by the selected target socket.
type ClientPreferenceStore interface {
	LoadLastUsed(socket string) (terminal.Client, bool, error)
	SaveLastUsed(socket string, client terminal.Client) error
}

// FileClientPreferenceStore writes preferences under the project's ignored
// .workspace directory. The hashed filename keeps arbitrary socket paths out
// of project paths while keeping each tmux server independent.
type FileClientPreferenceStore struct {
	ProjectRoot string
}

type clientPreferenceFile struct {
	Version int             `json:"version"`
	Socket  string          `json:"socket"`
	Client  terminal.Client `json:"client"`
}

func (s FileClientPreferenceStore) path(socket string) string {
	digest := sha256.Sum256([]byte(socket))
	return filepath.Join(s.ProjectRoot, ".workspace", "tui", "clients", "last-"+hex.EncodeToString(digest[:])+".json")
}

// LoadLastUsed is read-only. A missing file means this socket has no saved
// preference; it never creates the preference directory.
func (s FileClientPreferenceStore) LoadLastUsed(socket string) (terminal.Client, bool, error) {
	if s.ProjectRoot == "" {
		return terminal.Client{}, false, nil
	}
	data, err := os.ReadFile(s.path(socket))
	if errors.Is(err, os.ErrNotExist) {
		return terminal.Client{}, false, nil
	}
	if err != nil {
		return terminal.Client{}, false, err
	}
	var saved clientPreferenceFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return terminal.Client{}, false, fmt.Errorf("decode last-used tmux client: %w", err)
	}
	if saved.Version != 1 || saved.Socket != socket || saved.Client.TTY == "" {
		return terminal.Client{}, false, fmt.Errorf("last-used tmux client preference is invalid")
	}
	return saved.Client, true, nil
}

// SaveLastUsed atomically replaces this socket's UI-only preference.
func (s FileClientPreferenceStore) SaveLastUsed(socket string, client terminal.Client) error {
	if s.ProjectRoot == "" {
		return fmt.Errorf("project root is unavailable for saving the last-used tmux client")
	}
	if client.TTY == "" {
		return fmt.Errorf("cannot save an empty tmux client identity")
	}
	path := s.path(socket)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create TUI preference directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".last-client-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary TUI preference: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set TUI preference permissions: %w", err)
	}
	data, err := json.MarshalIndent(clientPreferenceFile{Version: 1, Socket: socket, Client: client}, "", "  ")
	if err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode last-used tmux client: %w", err)
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write last-used tmux client: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync last-used tmux client: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close last-used tmux client: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace last-used tmux client: %w", err)
	}
	return nil
}

func orderedClients(clients []terminal.Client, last terminal.Client, hasLast bool) ([]terminal.Client, bool) {
	ordered := append([]terminal.Client(nil), clients...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.TTY != b.TTY {
			return a.TTY < b.TTY
		}
		if a.Session != b.Session {
			return a.Session < b.Session
		}
		if a.PID != b.PID {
			return a.PID < b.PID
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Created < b.Created
	})
	for i := range ordered {
		if hasLast && last.SameClient(ordered[i]) {
			preferred := ordered[i]
			copy(ordered[1:i+1], ordered[0:i])
			ordered[0] = preferred
			return ordered, true
		}
	}
	return ordered, false
}
