package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Store keeps the current settings in memory and persists them to a JSON
// file. It is safe for concurrent use.
type Store struct {
	path string

	mu   sync.RWMutex
	cur  Settings
	subs []chan struct{}
}

// Open loads settings from path, creating the file with Default() settings
// when it does not exist.
func Open(path string) (*Store, error) {
	st := &Store{path: path}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		st.cur = Default()
		if err := st.save(st.cur); err != nil {
			return nil, err
		}
		return st, nil
	case err != nil:
		return nil, err
	}

	s := Default()
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid settings in %s: %w", path, err)
	}
	st.cur = s
	return st, nil
}

// Get returns a copy of the current settings.
func (st *Store) Get() Settings {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.cur.Clone()
}

// Update applies fn to a copy of the current settings, validates and saves
// the result, then notifies subscribers. If fn or validation fails nothing
// changes.
func (st *Store) Update(fn func(*Settings) error) (Settings, error) {
	st.mu.Lock()
	next := st.cur.Clone()
	if err := fn(&next); err != nil {
		st.mu.Unlock()
		return Settings{}, err
	}
	if err := next.Validate(); err != nil {
		st.mu.Unlock()
		return Settings{}, &ValidationError{Err: err}
	}
	if err := st.save(next); err != nil {
		st.mu.Unlock()
		return Settings{}, err
	}
	st.cur = next
	subs := append([]chan struct{}(nil), st.subs...)
	st.mu.Unlock()

	for _, c := range subs {
		select {
		case c <- struct{}{}:
		default: // a notification is already pending
		}
	}
	return next.Clone(), nil
}

// Subscribe returns a channel that receives a value after each successful
// Update. Notifications are coalesced: a slow reader sees at most one
// pending value.
func (st *Store) Subscribe() <-chan struct{} {
	c := make(chan struct{}, 1)
	st.mu.Lock()
	st.subs = append(st.subs, c)
	st.mu.Unlock()
	return c
}

func (st *Store) save(s Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(st.path, append(b, '\n'), 0o600)
}

// ValidationError wraps errors caused by invalid input, as opposed to I/O.
type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

// WriteFileAtomic writes data to a temporary file in the same directory and
// renames it over path, so readers never see a partial file.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
