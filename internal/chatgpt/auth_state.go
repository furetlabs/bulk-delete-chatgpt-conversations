package chatgpt

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const authFileEnvVar = "CHATGPT_BULK_AUTH_FILE"

type AuthState struct {
	SessionToken string    `json:"session_token"`
	CSRFToken    string    `json:"csrf_token,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	UserEmail    string    `json:"user_email,omitempty"`
	SavedAt      time.Time `json:"saved_at"`
	Source       string    `json:"source,omitempty"`
}

type AuthSource string

const (
	AuthSourceNone   AuthSource = "none"
	AuthSourceStored AuthSource = "stored"
)

type ResolvedAuth struct {
	State  AuthState
	Source AuthSource
	Path   string
}

func ResolveAuthStatePath() (string, error) {
	if override := strings.TrimSpace(os.Getenv(authFileEnvVar)); override != "" {
		return filepath.Clean(override), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(configDir, "chatgpt-bulk", "auth.json"), nil
}

const keyringService = "chatgpt-cleaner"

func keyringAccount(path string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(path))) }

func LoadAuthState() (AuthState, string, error) {
	path, err := ResolveAuthStatePath()
	if err != nil {
		return AuthState{}, "", err
	}
	raw, err := keyring.Get(keyringService, keyringAccount(path))
	if errors.Is(err, keyring.ErrNotFound) {
		return AuthState{}, path, os.ErrNotExist
	}
	if err != nil {
		return AuthState{}, path, fmt.Errorf("access secure credential store: %w", err)
	}
	var state AuthState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return AuthState{}, path, fmt.Errorf("decode secure auth state: %w", err)
	}
	if strings.TrimSpace(state.SessionToken) == "" {
		return AuthState{}, path, errors.New("stored auth is missing a session token")
	}
	return state, path, nil
}

func SaveAuthState(state AuthState) (string, error) {
	path, err := ResolveAuthStatePath()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state.SessionToken) == "" {
		return "", errors.New("cannot save auth state without a session token")
	}
	if state.SavedAt.IsZero() {
		state.SavedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	if err = keyring.Set(keyringService, keyringAccount(path), string(raw)); err != nil {
		return "", fmt.Errorf("save secure auth state: %w", err)
	}
	return path, nil
}

func RemoveAuthState() (string, bool, error) {
	path, err := ResolveAuthStatePath()
	if err != nil {
		return "", false, err
	}
	err = keyring.Delete(keyringService, keyringAccount(path))
	if errors.Is(err, keyring.ErrNotFound) {
		return path, false, nil
	}
	if err != nil {
		return path, false, fmt.Errorf("remove secure auth state: %w", err)
	}
	return path, true, nil
}

// MigrateLegacyAuthState imports but never deletes the old file. Call only on explicit user request.
func MigrateLegacyAuthState() error {
	path, err := ResolveAuthStatePath()
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("legacy auth is not a regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state AuthState
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if _, _, err = LoadAuthState(); err == nil {
		return errors.New("secure auth already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = SaveAuthState(state)
	return err
}

func ResolveAuth() (ResolvedAuth, error) {
	if stored, storedPath, err := LoadAuthState(); err == nil {
		return ResolvedAuth{State: stored, Source: AuthSourceStored, Path: storedPath}, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ResolvedAuth{}, err
	}

	path, err := ResolveAuthStatePath()
	if err != nil {
		return ResolvedAuth{}, err
	}

	return ResolvedAuth{Source: AuthSourceNone, Path: path}, nil
}

func MaskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "Unavailable"
	}
	return "[redacted]"
}
