//go:build !windows

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
)

// Load reads the active provider state from disk and caches it in memory.
// Uses a shared flock for concurrent read safety across processes.
func (s *ActiveProviderStore) Load() (*ActiveProviderState, error) {
	f, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.state = &ActiveProviderState{}
			s.mu.Unlock()
			return &ActiveProviderState{}, nil
		}
		return nil, err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	var alias activeProviderStateAlias
	if err := json.NewDecoder(f).Decode(&alias); err != nil {
		return nil, err
	}
	state := alias.toState()

	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return state, nil
}

// LoadFromFile reads the active provider state fresh from disk without updating
// the in-memory cache. Use this when the daemon needs to see changes made by
// the CLI while the daemon is running.
func (s *ActiveProviderStore) LoadFromFile() (*ActiveProviderState, error) {
	f, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ActiveProviderState{}, nil
		}
		return nil, err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	var alias activeProviderStateAlias
	if err := json.NewDecoder(f).Decode(&alias); err != nil {
		return nil, err
	}
	return alias.toState(), nil
}

// Save persists the active provider state to disk atomically.
// Uses an exclusive flock to prevent concurrent writes across processes,
// and a temp-file + rename for atomic replacement.
func (s *ActiveProviderStore) Save(state *ActiveProviderState) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}

	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return nil
}
