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
		if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE substr(name,1,7) != 'sqlite_'").Scan(&count); err != nil {
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
		if err = validateSchema(ctx, conn); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported archive schema version %d (supported: 1)", version)
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

// validateSchema checks the properties needed by inserts, conflict handling,
// and cascading deletion. It only reads schema metadata and never repairs it.
func validateSchema(ctx context.Context, conn *sql.Conn) error {
	for _, q := range schemaChecks {
		rows, err := conn.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("incomplete archive schema: %w", err)
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	for _, table := range []string{"pages", "captures", "blocks"} {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("incomplete archive schema: %s is not a table", table)
		}
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info(?) WHERE pk>0`, table).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("incomplete archive schema: %s primary key", table)
		}
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info(?) WHERE name='id' AND upper(type)='INTEGER' AND pk=1`, table).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("incomplete archive schema: %s id", table)
		}
		// A rowid-alias INTEGER PRIMARY KEY has no separate primary-key index.
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_index_list(?) WHERE origin='pk'`, table).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return fmt.Errorf("incomplete archive schema: %s id is not a rowid alias", table)
		}
	}
	for _, key := range []struct {
		table, columns string
		count          int
	}{{"pages", "url", 1}, {"captures", "page_id,content_hash", 2}, {"blocks", "page_id,block_hash", 2}} {
		var n int
		err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_index_list(?) AS i WHERE i."unique"=1 AND i.partial=0 AND (SELECT count(*) FROM pragma_index_info(i.name))=? AND (SELECT group_concat(name, ',') FROM pragma_index_info(i.name))=? AND NOT EXISTS (SELECT 1 FROM pragma_index_info(i.name) WHERE cid<0 OR name IS NULL) AND NOT EXISTS (SELECT 1 FROM pragma_index_xinfo(i.name) WHERE "key"=1 AND coll != 'BINARY')`, key.table, key.count, key.columns).Scan(&n)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("incomplete archive schema: %s unique %s", key.table, key.columns)
		}
	}
	for _, table := range []string{"captures", "blocks"} {
		var n int
		err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_list(?) AS f WHERE f."table"='pages' AND f."from"='page_id' AND f."to"='id' AND f.on_delete='CASCADE' AND f.seq=0 AND NOT EXISTS (SELECT 1 FROM pragma_foreign_key_list(?) AS other WHERE other.id=f.id AND other.seq!=0)`, table, table).Scan(&n)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("incomplete archive schema: %s page cascade", table)
		}
	}
	return nil
}
