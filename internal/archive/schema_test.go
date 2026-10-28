package archive

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestRejectForeignAndIncompleteWithoutMutation(t *testing.T) {
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "foreign.sqlite")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(fmt.Sprintf("CREATE TABLE untouched(value TEXT); INSERT INTO untouched VALUES('preserved'); PRAGMA user_version=%d", version)); err != nil {
				t.Fatal(err)
			}
			db.Close()
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s, err := Open(path)
			if err == nil {
				s.Close()
				t.Fatal("accepted foreign or incomplete archive")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("archive bytes changed")
			}
			db, err = sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var mode string
			if err = db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "delete" {
				t.Fatalf("journal changed: %s %v", mode, err)
			}
		})
	}
}

func TestForeignSQLitePrefixSchemaZeroIsNotInitialized(t *testing.T) {
	assertRejectedSchemaUnchanged(t, `CREATE TABLE sqliteForeign(value TEXT); INSERT INTO sqliteForeign VALUES('preserved');`)
}

func TestSchemaOneRequiresWriteAndCascadeInvariants(t *testing.T) {
	const schema = `CREATE TABLE pages(id INTEGER PRIMARY KEY,url TEXT NOT NULL UNIQUE,title TEXT NOT NULL,hostname TEXT NOT NULL,first_seen TEXT NOT NULL,last_seen TEXT NOT NULL,latest_capture_id INTEGER);
 CREATE TABLE captures(id INTEGER PRIMARY KEY,page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,captured_at TEXT NOT NULL,content_hash TEXT NOT NULL,block_count INTEGER NOT NULL,reason TEXT NOT NULL,payload_json TEXT NOT NULL,UNIQUE(page_id,content_hash));
 CREATE TABLE blocks(id INTEGER PRIMARY KEY,page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,block_hash TEXT NOT NULL,payload_json TEXT NOT NULL,search_text TEXT NOT NULL,UNIQUE(page_id,block_hash)); PRAGMA user_version=1;`
	cases := []struct{ name, from, to string }{
		{"pages primary key", "pages(id INTEGER PRIMARY KEY", "pages(id INTEGER"},
		{"non-rowid pages key", "pages(id INTEGER PRIMARY KEY", "pages(id INTEGER PRIMARY KEY DESC"},
		{"non-rowid capture key", "captures(id INTEGER PRIMARY KEY", "captures(id INTEGER PRIMARY KEY DESC"},
		{"non-rowid block key", "blocks(id INTEGER PRIMARY KEY", "blocks(id INTEGER PRIMARY KEY DESC"},
		{"captures primary key", "captures(id INTEGER PRIMARY KEY", "captures(id INTEGER"},
		{"blocks primary key", "blocks(id INTEGER PRIMARY KEY", "blocks(id INTEGER"},
		{"page URL unique", "url TEXT NOT NULL UNIQUE", "url TEXT NOT NULL"},
		{"capture hash unique", "UNIQUE(page_id,content_hash)", "UNIQUE(content_hash)"},
		{"block hash unique", "UNIQUE(page_id,block_hash)", "UNIQUE(block_hash)"},
		{"capture foreign key", "page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,captured_at", "page_id INTEGER NOT NULL,captured_at"},
		{"block foreign key", "page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,block_hash", "page_id INTEGER NOT NULL,block_hash"},
		{"capture cascade", "REFERENCES pages(id) ON DELETE CASCADE,captured_at", "REFERENCES pages(id),captured_at"},
		{"block cascade", "REFERENCES pages(id) ON DELETE CASCADE,block_hash", "REFERENCES pages(id),block_hash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(schema, tc.from) {
				t.Fatal("invalid test fixture")
			}
			assertRejectedSchemaUnchanged(t, strings.Replace(schema, tc.from, tc.to, 1))
		})
	}
}

func assertRejectedSchemaUnchanged(t *testing.T, schema string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err == nil {
		s.Close()
		t.Fatal("accepted incompatible schema")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("database bytes changed")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	if err = db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "delete" {
		t.Fatalf("journal changed: %s %v", mode, err)
	}
}
