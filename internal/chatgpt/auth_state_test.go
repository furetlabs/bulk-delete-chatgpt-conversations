package chatgpt

import (
	"github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveAuthStatePathOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom-auth.json")
	t.Setenv(authFileEnvVar, override)

	path, err := ResolveAuthStatePath()
	if err != nil {
		t.Fatalf("ResolveAuthStatePath() error = %v", err)
	}
	if path != override {
		t.Fatalf("ResolveAuthStatePath() = %q, want %q", path, override)
	}
}

func TestSaveLoadAndRemoveAuthState(t *testing.T) {
	keyring.MockInit()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, authPath)

	savedAt := time.Date(2026, time.March, 24, 10, 30, 0, 0, time.UTC)
	state := AuthState{
		SessionToken: "session-token-value",
		CSRFToken:    "csrf-token-value",
		AccessToken:  "access-token-value",
		UserEmail:    "user@example.com",
		SavedAt:      savedAt,
		Source:       "browser",
	}

	path, err := SaveAuthState(state)
	if err != nil {
		t.Fatalf("SaveAuthState() error = %v", err)
	}
	if path != authPath {
		t.Fatalf("SaveAuthState() path = %q, want %q", path, authPath)
	}

	loaded, loadedPath, err := LoadAuthState()
	if err != nil {
		t.Fatalf("LoadAuthState() error = %v", err)
	}
	if loadedPath != authPath {
		t.Fatalf("LoadAuthState() path = %q, want %q", loadedPath, authPath)
	}
	if loaded != state {
		t.Fatalf("LoadAuthState() = %#v, want %#v", loaded, state)
	}

	removedPath, removed, err := RemoveAuthState()
	if err != nil {
		t.Fatalf("RemoveAuthState() error = %v", err)
	}
	if removedPath != authPath {
		t.Fatalf("RemoveAuthState() path = %q, want %q", removedPath, authPath)
	}
	if !removed {
		t.Fatalf("RemoveAuthState() removed = false, want true")
	}
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Fatalf("auth file still exists after RemoveAuthState(): stat err = %v", err)
	}
}

func TestResolveAuthReturnsStoredAuth(t *testing.T) {
	keyring.MockInit()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, authPath)

	_, err := SaveAuthState(AuthState{
		SessionToken: "stored-session",
		CSRFToken:    "stored-csrf",
		AccessToken:  "stored-access",
		UserEmail:    "stored@example.com",
		SavedAt:      time.Date(2026, time.March, 24, 11, 0, 0, 0, time.UTC),
		Source:       "browser",
	})
	if err != nil {
		t.Fatalf("SaveAuthState() error = %v", err)
	}

	resolved, err := ResolveAuth()
	if err != nil {
		t.Fatalf("ResolveAuth() error = %v", err)
	}
	if resolved.Source != AuthSourceStored {
		t.Fatalf("ResolveAuth() source = %q, want %q", resolved.Source, AuthSourceStored)
	}
	if resolved.State.SessionToken != "stored-session" {
		t.Fatalf("ResolveAuth() session token = %q, want stored value", resolved.State.SessionToken)
	}
}

func TestResolveAuthReturnsNoneWhenAuthFileMissing(t *testing.T) {
	keyring.MockInit()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, authPath)

	resolved, err := ResolveAuth()
	if err != nil {
		t.Fatalf("ResolveAuth() error = %v", err)
	}
	if resolved.Source != AuthSourceNone {
		t.Fatalf("ResolveAuth() source = %q, want %q", resolved.Source, AuthSourceNone)
	}
	if resolved.Path != authPath {
		t.Fatalf("ResolveAuth() path = %q, want %q", resolved.Path, authPath)
	}
}

func TestResolveAuthReturnsStoredDecodeError(t *testing.T) {
	keyring.MockInit()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, authPath)

	if err := keyring.Set(keyringService, keyringAccount(authPath), "{not-json"); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", authPath, err)
	}

	_, err := ResolveAuth()
	if err == nil {
		t.Fatalf("ResolveAuth() error = nil, want decode failure")
	}
	if got := err.Error(); got == "" || got == os.ErrNotExist.Error() {
		t.Fatalf("ResolveAuth() error = %v, want decode failure", err)
	}
}

func TestMaskTokenNeverExposesSecret(t *testing.T) {
	for _, token := range []string{"short", "01234567890123456789012345"} {
		if MaskToken(token) != "[redacted]" {
			t.Error("token must be redacted")
		}
	}
}

func TestSaveAuthStateDoesNotFollowSymlink(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auth.json")
	if err := os.Symlink(target, path); err != nil {
		t.Skip(err)
	}
	t.Setenv(authFileEnvVar, path)
	if _, err := SaveAuthState(AuthState{SessionToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "untouched" {
		t.Fatal("target changed")
	}
}

func TestSaveAuthStateReplacesUnsafePermissions(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(authFileEnvVar, path)
	if _, err := SaveAuthState(AuthState{SessionToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "old" {
		t.Fatal("legacy file modified")
	}
}

func TestAuthStateIsNotWrittenToPlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, path)
	keyring.MockInit()
	if _, err := SaveAuthState(AuthState{SessionToken: "secret-value"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("plaintext auth file was written")
	}
}

func TestMigrationIsExplicitAndRetainsLegacyFile(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, path)
	if err := os.WriteFile(path, []byte(`{"session_token":"example"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadAuthState(); !os.IsNotExist(err) {
		t.Fatalf("legacy file was read implicitly: %v", err)
	}
	if err := MigrateLegacyAuthState(); err != nil {
		t.Fatal(err)
	}
	state, _, err := LoadAuthState()
	if err != nil || state.SessionToken != "example" {
		t.Fatalf("migration failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("legacy file unexpectedly removed")
	}
	if err := MigrateLegacyAuthState(); err == nil {
		t.Fatal("overwrote existing secure entry")
	}
}
