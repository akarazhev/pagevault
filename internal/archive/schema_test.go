package archive

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func TestUnsupportedSchemaVersionIsRejectedWithoutChanges(t *testing.T) {
	for _, version := range []int{2, 99, -1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "db")
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			if _, e = db.Exec(fmt.Sprintf("PRAGMA user_version=%d; CREATE TABLE untouched(value TEXT); INSERT INTO untouched VALUES('preserved');", version)); e != nil {
				t.Fatal(e)
			}
			s, e := Open(path)
			if e == nil {
				s.Close()
				t.Fatal("accepted unsupported schema", version)
			}
			var actual, pages int
			if e = db.QueryRow("PRAGMA user_version").Scan(&actual); e != nil || actual != version {
				t.Fatal("schema version changed", actual, e)
			}
			if e = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='pages'").Scan(&pages); e != nil || pages != 0 {
				t.Fatal("schema was initialized despite unsupported version", pages, e)
			}
			var value string
			if e = db.QueryRow("SELECT value FROM untouched").Scan(&value); e != nil || value != "preserved" {
				t.Fatal(value, e)
			}
		})
	}
}
func TestFreshSchemaIsInitializedAsVersionOne(t *testing.T) {
	s := store(t)
	var version int
	if e := s.DB.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != 1 {
		t.Fatal(version, e)
	}
}
