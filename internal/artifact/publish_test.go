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
