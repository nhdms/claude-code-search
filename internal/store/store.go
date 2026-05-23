package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strconv"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

func init() {
	sqlite_vec.Auto()
}

type DB struct {
	*sql.DB
	Dim int
}

func Open(path string, dim int) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	db := &DB{DB: conn, Dim: dim}
	if err := db.ensureVecTable(dim); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

// ResetEmbeddings drops the vec table, clears the stored dim, and marks all
// chunks as un-embedded. Use when switching to a model with a different dim.
func (d *DB) ResetEmbeddings(newDim int) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DROP TABLE IF EXISTS vec_chunks`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE chunks SET embedded=0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM meta WHERE k='embed_dim'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(k,v) VALUES('embed_dim',?)`, strconv.Itoa(newDim)); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`CREATE VIRTUAL TABLE vec_chunks USING vec0(embedding float[%d])`, newDim)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	d.Dim = newDim
	return nil
}

func (d *DB) ensureVecTable(dim int) error {
	var stored string
	_ = d.QueryRow(`SELECT v FROM meta WHERE k='embed_dim'`).Scan(&stored)
	if stored != "" {
		n, _ := strconv.Atoi(stored)
		if n != dim {
			return fmt.Errorf("embedding dim mismatch: db has %d, requested %d", n, dim)
		}
	} else {
		_, err := d.Exec(`INSERT INTO meta(k,v) VALUES('embed_dim',?)`, strconv.Itoa(dim))
		if err != nil {
			return err
		}
	}
	stmt := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
		embedding float[%d]
	)`, dim)
	_, err := d.Exec(stmt)
	return err
}

func (d *DB) GetSyncState(file string) (offset int64, mtime int64, err error) {
	row := d.QueryRow(`SELECT last_offset, last_mtime FROM sync_state WHERE file_path=?`, file)
	err = row.Scan(&offset, &mtime)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

func (d *DB) SetSyncState(file string, offset, mtime int64) error {
	_, err := d.Exec(`INSERT INTO sync_state(file_path,last_offset,last_mtime,last_synced_at)
		VALUES(?,?,?,datetime('now'))
		ON CONFLICT(file_path) DO UPDATE SET last_offset=excluded.last_offset, last_mtime=excluded.last_mtime, last_synced_at=excluded.last_synced_at`,
		file, offset, mtime)
	return err
}
