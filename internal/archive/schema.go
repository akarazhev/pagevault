package archive

import (
	"context"
	"database/sql"
	"fmt"
)

var schemaChecks = []string{
	"SELECT id,url,title,hostname,first_seen,last_seen,latest_capture_id FROM pages LIMIT 0",
	"SELECT id,page_id,captured_at,content_hash,block_count,reason,payload_json FROM captures LIMIT 0",
	"SELECT id,page_id,block_hash,payload_json,search_text FROM blocks LIMIT 0",
}

func prepareSchema(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "ROLLBACK")
	var version int
	if err = conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch version {
	case 0:
		var count int
		if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("unrecognized nonempty schema-0 database")
		}
		_, err = conn.ExecContext(ctx, `CREATE TABLE pages(id INTEGER PRIMARY KEY,url TEXT NOT NULL UNIQUE,title TEXT NOT NULL,hostname TEXT NOT NULL,first_seen TEXT NOT NULL,last_seen TEXT NOT NULL,latest_capture_id INTEGER);
 CREATE TABLE captures(id INTEGER PRIMARY KEY,page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,captured_at TEXT NOT NULL,content_hash TEXT NOT NULL,block_count INTEGER NOT NULL,reason TEXT NOT NULL,payload_json TEXT NOT NULL,UNIQUE(page_id,content_hash));
 CREATE TABLE blocks(id INTEGER PRIMARY KEY,page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,block_hash TEXT NOT NULL,payload_json TEXT NOT NULL,search_text TEXT NOT NULL,UNIQUE(page_id,block_hash));
 CREATE INDEX captures_page ON captures(page_id,id DESC); CREATE INDEX blocks_page ON blocks(page_id,id); CREATE INDEX pages_hostname ON pages(hostname); PRAGMA user_version=1;`)
		if err != nil {
			return err
		}
	case 1:
		for _, q := range schemaChecks {
			rows, e := conn.QueryContext(ctx, q)
			if e != nil {
				return fmt.Errorf("incomplete archive schema: %w", e)
			}
			if e = rows.Close(); e != nil {
				return e
			}
		}
	default:
		return fmt.Errorf("unsupported archive schema version %d (supported: 1)", version)
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
