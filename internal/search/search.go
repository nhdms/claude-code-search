package search

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nhduc/claude-search/internal/ingest"
	"github.com/nhduc/claude-search/internal/store"
)

type Opts struct {
	Query       string
	Limit       int
	Project     string
	Role        string
	Since       time.Time
	Embedder    *ingest.Embedder
	UseVector   bool
}

type Hit struct {
	ChunkID     int64   `json:"chunk_id"`
	MessageUUID string  `json:"message_uuid"`
	SessionID   string  `json:"session_id"`
	Role        string  `json:"role"`
	TS          string  `json:"ts"`
	Project     string  `json:"project"`
	Text        string  `json:"text"`
	FTSRank     int     `json:"fts_rank"`
	VecRank     int     `json:"vec_rank"`
	Score       float64 `json:"score"`
}

func Run(ctx context.Context, db *store.DB, opts Opts) ([]Hit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	per := 50

	ftsHits, err := ftsSearch(db, opts, per)
	if err != nil {
		return nil, fmt.Errorf("fts: %w", err)
	}

	var vecHits []Hit
	if opts.UseVector && opts.Embedder != nil {
		vecHits, err = vecSearch(ctx, db, opts, per)
		if err != nil {
			return nil, fmt.Errorf("vec: %w", err)
		}
	}

	merged := rrfMerge(ftsHits, vecHits, opts.Limit)
	return merged, nil
}

func ftsSearch(db *store.DB, opts Opts, k int) ([]Hit, error) {
	q := strings.TrimSpace(opts.Query)
	if q == "" {
		return nil, nil
	}
	ftsQ := buildFTSQuery(q)

	sb := strings.Builder{}
	args := []any{ftsQ}
	sb.WriteString(`SELECT c.id, c.message_uuid, c.session_id, c.role, c.ts, c.project_path, c.text
		FROM messages_fts f
		JOIN messages m ON m.uuid = f.uuid
		JOIN chunks c ON c.message_uuid = m.uuid
		WHERE messages_fts MATCH ?`)
	if opts.Role != "" {
		sb.WriteString(" AND m.role = ?")
		args = append(args, opts.Role)
	}
	if opts.Project != "" {
		sb.WriteString(" AND (m.cwd LIKE ? OR c.project_path LIKE ?)")
		p := "%" + opts.Project + "%"
		args = append(args, p, p)
	}
	if !opts.Since.IsZero() {
		sb.WriteString(" AND m.ts >= ?")
		args = append(args, opts.Since.Format(time.RFC3339))
	}
	sb.WriteString(" ORDER BY rank LIMIT ?")
	args = append(args, k)

	rows, err := db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	rank := 0
	seen := map[string]bool{}
	for rows.Next() {
		var h Hit
		var proj sql.NullString
		if err := rows.Scan(&h.ChunkID, &h.MessageUUID, &h.SessionID, &h.Role, &h.TS, &proj, &h.Text); err != nil {
			return nil, err
		}
		h.Project = proj.String
		if seen[h.MessageUUID] {
			continue
		}
		seen[h.MessageUUID] = true
		rank++
		h.FTSRank = rank
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func vecSearch(ctx context.Context, db *store.DB, opts Opts, k int) ([]Hit, error) {
	vecs, err := opts.Embedder.Embed(ctx, []string{opts.Query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	blob := ingest.Float32SliceToBlob(vecs[0])

	sb := strings.Builder{}
	args := []any{blob, k * 3}
	sb.WriteString(`SELECT c.id, c.message_uuid, c.session_id, c.role, c.ts, c.project_path, c.text, v.distance
		FROM vec_chunks v
		JOIN chunks c ON c.id = v.rowid
		JOIN messages m ON m.uuid = c.message_uuid
		WHERE v.embedding MATCH ? AND k = ?`)
	if opts.Role != "" {
		sb.WriteString(" AND m.role = ?")
		args = append(args, opts.Role)
	}
	if opts.Project != "" {
		sb.WriteString(" AND (m.cwd LIKE ? OR c.project_path LIKE ?)")
		p := "%" + opts.Project + "%"
		args = append(args, p, p)
	}
	if !opts.Since.IsZero() {
		sb.WriteString(" AND m.ts >= ?")
		args = append(args, opts.Since.Format(time.RFC3339))
	}
	sb.WriteString(" ORDER BY v.distance LIMIT ?")
	args = append(args, k)

	rows, err := db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	rank := 0
	seen := map[string]bool{}
	for rows.Next() {
		var h Hit
		var proj sql.NullString
		var dist float64
		if err := rows.Scan(&h.ChunkID, &h.MessageUUID, &h.SessionID, &h.Role, &h.TS, &proj, &h.Text, &dist); err != nil {
			return nil, err
		}
		h.Project = proj.String
		if seen[h.MessageUUID] {
			continue
		}
		seen[h.MessageUUID] = true
		rank++
		h.VecRank = rank
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func rrfMerge(fts, vec []Hit, limit int) []Hit {
	const kRRF = 60.0
	byKey := map[string]*Hit{}
	for _, h := range fts {
		hc := h
		byKey[hc.MessageUUID] = &hc
		byKey[hc.MessageUUID].Score = 1.0 / (kRRF + float64(hc.FTSRank))
	}
	for _, h := range vec {
		if existing, ok := byKey[h.MessageUUID]; ok {
			existing.VecRank = h.VecRank
			existing.Score += 1.0 / (kRRF + float64(h.VecRank))
		} else {
			hc := h
			hc.Score = 1.0 / (kRRF + float64(hc.VecRank))
			byKey[hc.MessageUUID] = &hc
		}
	}
	out := make([]Hit, 0, len(byKey))
	for _, h := range byKey {
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// buildFTSQuery escapes a user query for FTS5 by quoting each token,
// turning it into an OR/AND query of phrase matches. This avoids syntax errors
// from special chars in user queries.
func buildFTSQuery(q string) string {
	fields := strings.Fields(q)
	for i, f := range fields {
		f = strings.ReplaceAll(f, `"`, `""`)
		fields[i] = `"` + f + `"`
	}
	return strings.Join(fields, " ")
}
