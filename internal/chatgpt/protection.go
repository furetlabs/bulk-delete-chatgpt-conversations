package chatgpt

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var conversationIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func protectionPath() (string, error) {
	if p := os.Getenv("CHATGPT_BULK_PROTECTION_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "chatgpt-bulk", "protected.json"), nil
}
func protectionDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func readProtectionFile(path string) (map[string]bool, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("protection store is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var ids map[string]bool
	if err = json.Unmarshal(data, &ids); err != nil || ids == nil {
		return nil, nil, errors.New("invalid protection file")
	}
	for id, protected := range ids {
		if !conversationIDPattern.MatchString(id) || id != strings.ToLower(id) || !protected {
			return nil, nil, errors.New("invalid protection entry")
		}
	}
	return ids, data, nil
}

func readProtected() (map[string]bool, error) {
	path, err := protectionPath()
	if err != nil {
		return nil, err
	}
	ids, data, err := readProtectionFile(path)
	if err != nil {
		return nil, err
	}
	marker := path + ".initialized"
	info, err := os.Lstat(marker)
	if err != nil {
		return nil, fmt.Errorf("protection integrity marker: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("invalid protection integrity marker")
	}
	sum, err := os.ReadFile(marker)
	if err != nil {
		return nil, err
	}
	if string(sum) != protectionDigest(data) {
		return nil, errors.New("protection integrity check failed")
	}
	return ids, nil
}

// InitializeProtections imports an existing legacy store or creates a fresh one.
// A durable marker is written first, so interrupted initialization fails closed.
func InitializeProtections() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unlock, err := lockProtection(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	path, err := protectionPath()
	if err != nil {
		return err
	}
	marker := path + ".initialized"
	info, err := os.Lstat(marker)
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("invalid protection initialization marker")
		}
		_, err = readProtected()
		return err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err = os.Lstat(path); err == nil {
		return errors.New("existing unmarked protection store requires explicit import")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString("pending"); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = syncProtectionDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		if _, err = f.WriteString("{}"); err != nil {
			_ = f.Close()
			return err
		}
		if err = f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = syncProtectionDir(filepath.Dir(path)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	_, data, err := readProtectionFile(path)
	if err != nil {
		return err
	}
	return writeProtectionFile(marker, []byte(protectionDigest(data)))
}

// ImportLegacyProtections explicitly accepts an unmarked store without changing its contents.
func ImportLegacyProtections() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unlock, err := lockProtection(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	path, err := protectionPath()
	if err != nil {
		return err
	}
	if _, err = os.Lstat(path + ".initialized"); err == nil {
		return errors.New("protection marker already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, data, err := readProtectionFile(path)
	if err != nil {
		return err
	}
	return writeProtectionFile(path+".initialized", []byte(protectionDigest(data)))
}

// writeProtectionFile replaces one file atomically and persists the rename.
func writeProtectionFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".protected-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("save protections: %w", err)
	}
	return syncProtectionDir(filepath.Dir(path))
}

func IsProtected(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return IsProtectedContext(ctx, id)
}

func IsProtectedContext(ctx context.Context, id string) (bool, error) {
	unlock, err := lockProtection(ctx)
	if err != nil {
		return false, err
	}
	defer unlock()
	if !conversationIDPattern.MatchString(id) {
		return false, errors.New("invalid conversation ID")
	}
	ids, err := readProtected()
	return ids[strings.ToLower(id)], err
}
func SetProtected(id string, protect bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unlock, err := lockProtection(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if !conversationIDPattern.MatchString(id) {
		return errors.New("invalid conversation ID")
	}
	ids, err := readProtected()
	if err != nil {
		return err
	}
	id = strings.ToLower(id)
	if protect {
		ids[id] = true
	} else {
		delete(ids, id)
	}
	path, err := protectionPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	if err = writeProtectionFile(path, data); err != nil {
		return err
	}
	return writeProtectionFile(path+".initialized", []byte(protectionDigest(data)))
}

// Ensure a completed rename survives a system crash where the filesystem supports it.
func syncProtectionDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
