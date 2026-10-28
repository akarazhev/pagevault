package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishPairRestoresSymlinkAfterSecondRenameFails(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "release.zip")
	old := filepath.Join(dir, "old")
	sum := target + ".sha256"
	for path, content := range map[string]string{source: "new", old: "old artifact", sum: "old checksum"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(old, target); err != nil {
		t.Fatal(err)
	}
	rename := publishRename
	publishRename = func(from, to string) error {
		if to == sum && strings.HasPrefix(filepath.Base(from), ".pagevault-checksum-") {
			return errors.New("injected publication failure")
		}
		return os.Rename(from, to)
	}
	defer func() { publishRename = rename }()
	if _, err := PublishPair(source, target); err == nil {
		t.Fatal("expected publication failure")
	}
	link, err := os.Readlink(target)
	if err != nil || link != old {
		t.Fatalf("symlink not restored: %q %v", link, err)
	}
	for path, want := range map[string]string{old: "old artifact", sum: "old checksum"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", path, got, err)
		}
	}
}

func TestPublishPairReportsRetainedBackupWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "release.zip")
	sum := target + ".sha256"
	for path, content := range map[string]string{source: "new", target: "old artifact", sum: "old checksum"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rename := publishRename
	publishRename = func(from, to string) error {
		if to == sum && strings.HasPrefix(filepath.Base(from), ".pagevault-checksum-") {
			if err := os.Remove(target); err != nil {
				return err
			}
			if err := os.Mkdir(target, 0700); err != nil {
				return err
			}
			return errors.New("injected publication failure")
		}
		return os.Rename(from, to)
	}
	defer func() { publishRename = rename }()
	_, err := PublishPair(source, target)
	if err == nil {
		t.Fatal("expected publication and rollback failure")
	}
	backups, globErr := filepath.Glob(filepath.Join(dir, ".pagevault-backup-*"))
	if globErr != nil || len(backups) != 1 {
		t.Fatalf("expected retained backup: %v %v", backups, globErr)
	}
	if !strings.Contains(err.Error(), backups[0]) {
		t.Fatalf("error does not report backup path: %v", err)
	}
	got, readErr := os.ReadFile(backups[0])
	if readErr != nil || string(got) != "old artifact" {
		t.Fatalf("backup lost old bytes: %q %v", got, readErr)
	}
	checksum, readErr := os.ReadFile(sum)
	if readErr != nil || string(checksum) != "old checksum" {
		t.Fatalf("checksum not restored: %q %v", checksum, readErr)
	}
}
