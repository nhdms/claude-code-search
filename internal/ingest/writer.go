package ingest

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nhduc/claude-search/internal/store"
	"github.com/nhduc/claude-search/internal/transcript"
)

type ToolMode int

const (
	ToolNone  ToolMode = 0
	ToolSmall ToolMode = 1
	ToolAll   ToolMode = 2
)

type Writer struct {
	DB           *store.DB
	ToolMode     ToolMode
	ProjectPath  string
	NewChunks    int
	NewMessages  int
	SkippedDupes int
}

func NewWriter(db *store.DB, projectPath string, mode ToolMode) *Writer {
	return &Writer{DB: db, ProjectPath: projectPath, ToolMode: mode}
}

func (w *Writer) EnsureSession(tx *sql.Tx, sessionID, cwd, ts string) error {
	// project_path tracks the actual working directory (cwd) from the
	// transcript. The Claude projects/ directory name is ambiguous when paths
	// contain dashes (vibe/kanban vs vibe-kanban), so cwd from the event is
	// the only trustworthy source. If we have no cwd on THIS event (e.g.
	// ai-title), pass empty so the ON CONFLICT branch keeps the existing
	// stored value rather than clobbering it with a decoded guess.
	projectPath := cwd
	_, err := tx.Exec(`INSERT INTO sessions(id, project_path, cwd, started_at, ended_at, message_count)
		VALUES(?,?,?,?,?,0)
		ON CONFLICT(id) DO UPDATE SET
		  ended_at=excluded.ended_at,
		  cwd=COALESCE(NULLIF(excluded.cwd,''), sessions.cwd),
		  project_path=COALESCE(NULLIF(excluded.project_path,''), sessions.project_path)`,
		sessionID, projectPath, cwd, ts, ts)
	return err
}

func (w *Writer) WriteEvent(tx *sql.Tx, ev transcript.Event) error {
	if ev.Kind == "ai-title" {
		// Update session title; create row if missing.
		if err := w.EnsureSession(tx, ev.SessionID, ev.CWD, ev.Timestamp); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, ev.Title, ev.SessionID)
		return err
	}
	if err := w.EnsureSession(tx, ev.SessionID, ev.CWD, ev.Timestamp); err != nil {
		return err
	}

	var existing string
	err := tx.QueryRow(`SELECT uuid FROM messages WHERE uuid=?`, ev.UUID).Scan(&existing)
	if err == nil {
		w.SkippedDupes++
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}

	text := ev.Text
	if ev.Kind == "tool_use" {
		text = fmt.Sprintf("[tool_use %s] %s", ev.ToolName, truncate(ev.ToolInput, 4096))
	} else if ev.Kind == "tool_result" {
		switch w.ToolMode {
		case ToolNone:
			text = ""
		case ToolSmall:
			if len(ev.ToolOutput) > MaxToolOutputSize {
				text = ev.ToolOutput[:MaxToolOutputSize] + "\n[truncated]"
			} else {
				text = ev.ToolOutput
			}
		case ToolAll:
			text = ev.ToolOutput
		}
	}

	_, err = tx.Exec(`INSERT INTO messages(uuid, session_id, parent_uuid, role, kind, model, ts, cwd, text, tool_name, tool_input, tool_output)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		ev.UUID, ev.SessionID, ev.ParentUUID, ev.Role, ev.Kind, ev.Model, ev.Timestamp, ev.CWD,
		ev.Text, ev.ToolName, ev.ToolInput, ev.ToolOutput)
	if err != nil {
		return err
	}
	w.NewMessages++

	if strings.TrimSpace(text) == "" {
		return nil
	}

	_, err = tx.Exec(`INSERT INTO messages_fts(text, session_id, uuid, role, ts) VALUES(?,?,?,?,?)`,
		text, ev.SessionID, ev.UUID, ev.Role, ev.Timestamp)
	if err != nil {
		return err
	}

	for _, c := range ChunkText(text) {
		_, err = tx.Exec(`INSERT INTO chunks(message_uuid, session_id, project_path, role, ts, text, embedded)
			VALUES(?,?,?,?,?,?,0)`,
			ev.UUID, ev.SessionID, w.ProjectPath, ev.Role, ev.Timestamp, c)
		if err != nil {
			return err
		}
		w.NewChunks++
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// DecodeProjectPath converts Claude's flattened project dir name back to a path.
// e.g. "-Users-nhduc-wp-hd-orchestrator" -> "/Users/nhduc/wp/hd/orchestrator"
func DecodeProjectPath(dirName string) string {
	dirName = filepath.Base(dirName)
	if strings.HasPrefix(dirName, "-") {
		return strings.ReplaceAll(dirName, "-", "/")
	}
	return dirName
}
