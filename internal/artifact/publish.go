// Package artifact publishes an artifact and its checksum as a recoverable pair.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PublishPair stages both files and rolls back ordinary publication failures.
// It does not provide crash atomicity.
func PublishPair(source, output string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return "", err
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	file, err := os.CreateTemp(filepath.Dir(output), ".pagevault-artifact-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(file, hash), in)
	if err == nil {
		err = file.Chmod(0644)
	}
	if e := file.Close(); err == nil {
		err = e
	}
	if err != nil {
		return "", err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	sum, err := os.CreateTemp(filepath.Dir(output), ".pagevault-checksum-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(sum.Name())
	_, err = fmt.Fprintf(sum, "%s  %s\n", digest, filepath.Base(output))
	if err == nil {
		err = sum.Chmod(0644)
	}
	if e := sum.Close(); err == nil {
		err = e
	}
	if err != nil {
		return "", err
	}
	targets := []string{output, output + ".sha256"}
	staged := []string{file.Name(), sum.Name()}
	backups := make([]string, 2)
	installed := make([]bool, 2)
	for _, target := range targets {
		if info, e := os.Lstat(target); e == nil && info.IsDir() {
			return "", fmt.Errorf("refusing to replace directory %s", target)
		} else if e != nil && !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
	}
	rollback := func(cause error) error {
		for i := len(targets) - 1; i >= 0; i-- {
			if backups[i] != "" {
				if e := os.Rename(backups[i], targets[i]); e != nil {
					cause = errors.Join(cause, fmt.Errorf("recovery backup retained at %s: %w", backups[i], e))
				} else {
					backups[i] = ""
				}
			} else if installed[i] {
				if e := os.Remove(targets[i]); e != nil && !errors.Is(e, os.ErrNotExist) {
					cause = errors.Join(cause, e)
				}
			}
		}
		return cause
	}
	for i, target := range targets {
		if _, e := os.Lstat(target); e == nil {
			b, e := os.CreateTemp(filepath.Dir(target), ".pagevault-backup-*")
			if e != nil {
				return "", rollback(e)
			}
			backup := b.Name()
			if e = b.Close(); e == nil {
				e = os.Remove(backup)
			}
			if e == nil {
				e = os.Rename(target, backup)
			}
			if e != nil {
				os.Remove(backup)
				return "", rollback(e)
			}
			backups[i] = backup
		} else if !errors.Is(e, os.ErrNotExist) {
			return "", rollback(e)
		}
		if e := publishRename(staged[i], target); e != nil {
			return "", rollback(e)
		}
		installed[i] = true
	}
	for _, backup := range backups {
		if backup != "" {
			_ = os.Remove(backup)
		}
	}
	return digest, nil
}

var publishRename = os.Rename
