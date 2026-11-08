package chatgpt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProtectionPersistsAndBlocksDeletion(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	if err := SetProtected(id, true); err != nil {
		t.Fatal(err)
	}
	protected, err := IsProtected(id)
	if err != nil || !protected {
		t.Fatalf("protection lost: %v", err)
	}
	c := &Client{allowMutations: true}
	if err := c.DeleteConversation(context.Background(), id); err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatal("protected deletion was permitted")
	}
}
func TestDeleteRejectsUnidentifiedConversation(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	c := &Client{allowMutations: true}
	if err := c.DeleteConversation(context.Background(), ""); err == nil {
		t.Fatal("empty ID allowed")
	}
}

func TestDeletionFailsWhenProtectionStoreMissing(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "missing"))
	c := &Client{allowMutations: true}
	err := c.DeleteConversation(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err == nil || !strings.Contains(err.Error(), "protection") {
		t.Fatalf("expected protection failure, got %v", err)
	}
}

func TestMutationsDisabledByDefault(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protection.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.DeleteConversation(context.Background(), "11111111-1111-4111-8111-111111111111"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("delete should be disabled: %v", err)
	}
	if err = client.ArchiveConversation(context.Background(), "11111111-1111-4111-8111-111111111111"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("archive should be disabled: %v", err)
	}
}

func TestProtectionStoreConcurrentProcesses(t *testing.T) {
	path := os.Getenv("PROTECTION_TEST_PATH")
	if path != "" {
		id := os.Getenv("PROTECTION_TEST_ID")
		for i := 0; i < 20; i++ {
			if err := SetProtected(id, true); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	path = filepath.Join(t.TempDir(), "protected.json")
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", path)
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	var cmds []*exec.Cmd
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("%08x-1111-4111-8111-111111111111", i+1)
		cmd := exec.Command(os.Args[0], "-test.run=^TestProtectionStoreConcurrentProcesses$")
		cmd.Env = append(os.Environ(), "PROTECTION_TEST_PATH="+path, "PROTECTION_TEST_ID="+id)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("%08x-1111-4111-8111-111111111111", i+1)
		p, err := IsProtected(id)
		if err != nil || !p {
			t.Fatalf("lost protection %s: %v", id, err)
		}
	}
}

func TestProtectionStoreRejectsInvalidEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", path)
	for _, bad := range []string{`{"not-an-id":true}`, `{"11111111-1111-4111-8111-111111111111":false}`, `{"11111111-1111-4111-8111-111111111111":true,"oops":true}`} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := IsProtected("11111111-1111-4111-8111-111111111111"); err == nil {
			t.Fatalf("accepted invalid store %s", bad)
		}
	}
}

func TestArchiveDoesNotBypassAnotherOperationLock(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockProtection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := &Client{allowMutations: true, inventoryComplete: true}
	err = c.ArchiveConversation(ctx, "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected lock timeout: %v", err)
	}
}

func TestInitializeNeverRecreatesMissingExistingStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", path)
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	if err := SetProtected("11111111-1111-4111-8111-111111111111", true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := InitializeProtections(); err == nil {
		t.Fatal("missing protections recreated as empty")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("store recreated")
	}
}

func TestProtectedConversationCannotBeArchived(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	if err := SetProtected(id, true); err != nil {
		t.Fatal(err)
	}
	c := &Client{allowMutations: true, inventoryComplete: true, inventoryIDs: map[string]struct{}{id: {}}}
	if err := c.ArchiveConversation(context.Background(), id); err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("protected archive permitted: %v", err)
	}
}

func TestProtectionCheckHonorsCancellation(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockProtection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = IsProtectedContext(ctx, "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout: %v", err)
	}
}

func TestProtectionStoreRejectsUndetectedValidJSONChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", path)
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	if err := SetProtected(id, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := IsProtected(id); err == nil {
		t.Fatal("accepted modified protection file")
	}
}

func TestExistingUnmarkedProtectionsRequireExplicitImport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.json")
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", path)
	id := "11111111-1111-4111-8111-111111111111"
	if err := os.WriteFile(path, []byte(`{"`+id+`":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := InitializeProtections(); err == nil {
		t.Fatal("accepted unmarked store without explicit import")
	}
	if err := ImportLegacyProtections(); err != nil {
		t.Fatal(err)
	}
	p, err := IsProtected(id)
	if err != nil || !p {
		t.Fatalf("lost imported protection: %v", err)
	}
	if err := ImportLegacyProtections(); err == nil {
		t.Fatal("overwrote existing marker")
	}
}
