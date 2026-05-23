package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nhduc/claude-search/internal/store"
	"github.com/nhduc/claude-search/internal/transcript"
)

type ImportOpts struct {
	ProjectsDir string
	ToolMode    ToolMode
	OnlyProject string
}

type ImportStats struct {
	FilesScanned int
	NewMessages  int
	NewChunks    int
	Skipped      int
	Errors       int
}

func RunImport(ctx context.Context, db *store.DB, opts ImportOpts) (*ImportStats, error) {
	stats := &ImportStats{}
	entries, err := os.ReadDir(opts.ProjectsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if opts.OnlyProject != "" && filepath.Base(e.Name()) != opts.OnlyProject {
			continue
		}
		projDir := filepath.Join(opts.ProjectsDir, e.Name())
		projPath := DecodeProjectPath(e.Name())
		walkErr := filepath.WalkDir(projDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			if ierr := importFile(ctx, db, path, projPath, opts.ToolMode, stats); ierr != nil {
				fmt.Fprintf(os.Stderr, "warn: %s: %v\n", path, ierr)
				stats.Errors++
			}
			stats.FilesScanned++
			return nil
		})
		if walkErr != nil {
			stats.Errors++
		}
	}
	return stats, nil
}

func importFile(ctx context.Context, db *store.DB, path, projPath string, mode ToolMode, stats *ImportStats) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mtime := info.ModTime().Unix()
	offset, prevMtime, err := db.GetSyncState(path)
	if err != nil {
		return err
	}
	if mtime == prevMtime && offset == info.Size() {
		return nil
	}
	if mtime != prevMtime && offset > info.Size() {
		offset = 0
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	w := NewWriter(db, projPath, mode)
	var finalOffset int64 = offset
	finalOffset, err = transcript.ParseFile(path, offset, func(ev transcript.Event, newOffset int64) error {
		if err := w.WriteEvent(tx, ev); err != nil {
			return err
		}
		finalOffset = newOffset
		return ctx.Err()
	})
	if err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT INTO sync_state(file_path, last_offset, last_mtime, last_synced_at)
		VALUES(?,?,?,datetime('now'))
		ON CONFLICT(file_path) DO UPDATE SET last_offset=excluded.last_offset, last_mtime=excluded.last_mtime, last_synced_at=excluded.last_synced_at`,
		path, finalOffset, mtime)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	stats.NewMessages += w.NewMessages
	stats.NewChunks += w.NewChunks
	stats.Skipped += w.SkippedDupes
	return nil
}

func EmbedPending(ctx context.Context, db *store.DB, emb *Embedder, limit int) (int, error) {
	if emb == nil {
		return 0, fmt.Errorf("embedder is nil")
	}
	total := 0
	for {
		rows, err := db.Query(`SELECT id, text FROM chunks WHERE embedded=0 ORDER BY id LIMIT ?`, emb.Batch)
		if err != nil {
			return total, err
		}
		var ids []int64
		var texts []string
		for rows.Next() {
			var id int64
			var t string
			if err := rows.Scan(&id, &t); err != nil {
				rows.Close()
				return total, err
			}
			ids = append(ids, id)
			texts = append(texts, t)
		}
		rows.Close()
		if len(ids) == 0 {
			return total, nil
		}
		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			return total, err
		}
		tx, err := db.Begin()
		if err != nil {
			return total, err
		}
		for i, id := range ids {
			_, err = tx.Exec(`INSERT INTO vec_chunks(rowid, embedding) VALUES(?,?)`, id, Float32SliceToBlob(vecs[i]))
			if err != nil {
				_ = tx.Rollback()
				return total, err
			}
			_, err = tx.Exec(`UPDATE chunks SET embedded=1 WHERE id=?`, id)
			if err != nil {
				_ = tx.Rollback()
				return total, err
			}
		}
		if err := tx.Commit(); err != nil {
			return total, err
		}
		total += len(ids)
		if limit > 0 && total >= limit {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

// PendingChunks returns count of un-embedded chunks (for cost estimates).
func PendingChunks(db *store.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE embedded=0`).Scan(&n)
	return n, err
}
