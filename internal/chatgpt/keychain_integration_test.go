//go:build darwin && keychain_integration

package chatgpt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// This explicitly opted-in test uses only synthetic credentials and a random
// temporary profile. It does not open a browser or contact ChatGPT.
func TestNativeKeychainRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv(authFileEnvVar, path)
	state := AuthState{SessionToken: "synthetic-test-session", AccessToken: "synthetic-test-access"}
	if _, err := SaveAuthState(state); err != nil {
		t.Fatalf("save synthetic credential: %v", err)
	}
	defer func() {
		if _, _, err := RemoveAuthState(); err != nil {
			t.Errorf("cleanup synthetic credential: %v", err)
		}
	}()
	actual, _, err := LoadAuthState()
	if err != nil {
		t.Fatal(err)
	}
	if actual.SessionToken != state.SessionToken || actual.AccessToken != state.AccessToken {
		t.Fatal("credential round trip mismatch")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected legacy plaintext file: %v", err)
	}
}
