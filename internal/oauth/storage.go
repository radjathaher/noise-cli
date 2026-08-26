package oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func DefaultTokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "noise", "oauth-token.json"), nil
}

func (m *Manager) tokenPath() (string, error) {
	if m.cfg.TokenPath != "" {
		return m.cfg.TokenPath, nil
	}
	return DefaultTokenPath()
}

func (m *Manager) load() (Token, error) {
	path, err := m.tokenPath()
	if err != nil {
		return Token{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Token{}, err
	}
	defer file.Close()
	var token Token
	if err := json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&token); err != nil {
		return Token{}, fmt.Errorf("read OAuth token file: %w", err)
	}
	return token, nil
}

func (m *Manager) save(token Token) error {
	path, err := m.tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create OAuth token directory: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return writeTokenAtomically(path, append(data, '\n'))
}

func writeTokenAtomically(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".oauth-token-*")
	if err != nil {
		return fmt.Errorf("create OAuth token file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("save OAuth token: %w", err)
	}
	return os.Chmod(path, 0o600)
}
