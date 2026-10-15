package people

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileStore keeps labels in a JSON file that only the user can read. It
// holds account IDs and labels, never names or emails.
type FileStore struct {
	path string
}

// NewFileStore keeps labels in the file at path, created when needed.
func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

type labelFile struct {
	Labels map[string]string `json:"labels"` // account ID -> label
	Next   int               `json:"next"`
}

func (s *FileStore) read() (labelFile, error) {
	f := labelFile{Labels: map[string]string{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("people: %s: %w", s.path, err)
	}
	if f.Labels == nil {
		f.Labels = map[string]string{}
	}
	return f, nil
}

// Label implements LabelStore. The file is locked while a label is given
// out, so programs sharing it agree.
func (s *FileStore) Label(accountID string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return "", err
	}
	unlock, err := lock(s.path + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	f, err := s.read()
	if err != nil {
		return "", err
	}
	if l, ok := f.Labels[accountID]; ok {
		return l, nil
	}
	f.Next++
	l := "Person " + letters(f.Next)
	f.Labels[accountID] = l
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	return l, os.Rename(tmp, s.path)
}

// Account implements LabelStore.
func (s *FileStore) Account(label string) (string, bool) {
	f, err := s.read()
	if err != nil {
		return "", false
	}
	for id, l := range f.Labels {
		if strings.EqualFold(l, label) {
			return id, true
		}
	}
	return "", false
}
