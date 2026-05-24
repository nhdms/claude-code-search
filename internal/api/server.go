package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nhduc/claude-search/internal/ingest"
	"github.com/nhduc/claude-search/internal/search"
	"github.com/nhduc/claude-search/internal/store"
)

type Server struct {
	DB          *store.DB
	ProjectsDir string

	mu         sync.RWMutex
	apiKey     string
	embedModel string
	embedDim   int
	baseURL    string
	embed      *ingest.Embedder

	Jobs *Jobs
}

func New(db *store.DB, embedModel string, embedDim int, projectsDir string) *Server {
	s := &Server{
		DB: db, ProjectsDir: projectsDir,
		embedModel: embedModel, embedDim: embedDim,
		Jobs: NewJobs(),
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		s.apiKey = key
	}
	s.rebuildEmbedder()
	return s
}

func (s *Server) rebuildEmbedder() {
	if s.apiKey == "" && s.baseURL == "" {
		s.embed = nil
		return
	}
	key := s.apiKey
	if key == "" {
		key = "local"
	}
	s.embed = ingest.NewEmbedderWithBase(key, s.embedModel, s.embedDim, s.baseURL)
}

func (s *Server) setKey(k string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKey = k
	s.rebuildEmbedder()
}

func (s *Server) embedder() *ingest.Embedder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embed
}

func (s *Server) hasKey() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKey != ""
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/timeseries", s.timeseries)
	mux.HandleFunc("GET /api/projects", s.projects)
	mux.HandleFunc("GET /api/tools", s.tools)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.sessionDetail)
	mux.HandleFunc("POST /api/sessions/{id}/reindex", s.reindexSession)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("POST /api/resume", s.resume)

	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("POST /api/config", s.postConfig)
	mux.HandleFunc("POST /api/import", s.startImport)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)

	return cors(mux)
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	var sess, msgs, chunks, embedded int
	var firstTS, lastTS string
	s.DB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sess)
	s.DB.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgs)
	s.DB.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunks)
	s.DB.QueryRow(`SELECT COUNT(*) FROM chunks WHERE embedded=1`).Scan(&embedded)
	s.DB.QueryRow(`SELECT COALESCE(MIN(ts),''), COALESCE(MAX(ts),'') FROM messages`).Scan(&firstTS, &lastTS)

	roleRows, _ := s.DB.Query(`SELECT role, COUNT(*) FROM messages GROUP BY role`)
	roles := map[string]int{}
	if roleRows != nil {
		for roleRows.Next() {
			var role string
			var n int
			roleRows.Scan(&role, &n)
			roles[role] = n
		}
		roleRows.Close()
	}

	writeJSON(w, 200, map[string]any{
		"sessions":     sess,
		"messages":     msgs,
		"chunks":       chunks,
		"embedded":     embedded,
		"first_ts":     firstTS,
		"last_ts":      lastTS,
		"roles":        roles,
		"vector_ready": s.hasKey(),
		"projects_dir": s.ProjectsDir,
	})
}

func (s *Server) timeseries(w http.ResponseWriter, r *http.Request) {
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "day"
	}
	var fmtStr string
	switch bucket {
	case "hour":
		fmtStr = "%Y-%m-%dT%H:00"
	case "month":
		fmtStr = "%Y-%m"
	default:
		fmtStr = "%Y-%m-%d"
	}
	rows, err := s.DB.Query(fmt.Sprintf(`SELECT strftime('%s', ts) AS b, role, COUNT(*) FROM messages GROUP BY b, role ORDER BY b`, fmtStr))
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer rows.Close()
	type point struct {
		Bucket    string `json:"bucket"`
		User      int    `json:"user"`
		Assistant int    `json:"assistant"`
		Total     int    `json:"total"`
	}
	byBucket := map[string]*point{}
	var order []string
	for rows.Next() {
		var b, role string
		var n int
		rows.Scan(&b, &role, &n)
		p, ok := byBucket[b]
		if !ok {
			p = &point{Bucket: b}
			byBucket[b] = p
			order = append(order, b)
		}
		switch role {
		case "user":
			p.User += n
		case "assistant":
			p.Assistant += n
		}
		p.Total += n
	}
	out := make([]point, 0, len(order))
	for _, b := range order {
		out = append(out, *byBucket[b])
	}
	writeJSON(w, 200, out)
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var rows interface {
		Next() bool
		Scan(...any) error
		Close() error
	}
	var err error
	if q != "" {
		rows, err = s.DB.Query(`SELECT COALESCE(project_path, cwd, '(unknown)') AS p, COUNT(*) AS n
			FROM sessions
			WHERE (project_path LIKE ? OR cwd LIKE ?)
			GROUP BY p ORDER BY n DESC`, "%"+q+"%", "%"+q+"%")
	} else {
		rows, err = s.DB.Query(`SELECT COALESCE(project_path, cwd, '(unknown)') AS p, COUNT(*) AS n
			FROM sessions GROUP BY p ORDER BY n DESC`)
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer rows.Close()
	type proj struct {
		Path     string `json:"path"`
		Sessions int    `json:"sessions"`
	}
	out := []proj{}
	for rows.Next() {
		var p proj
		rows.Scan(&p.Path, &p.Sessions)
		out = append(out, p)
	}
	writeJSON(w, 200, out)
}

func (s *Server) tools(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`SELECT tool_name, COUNT(*) FROM messages WHERE kind='tool_use' AND tool_name != '' GROUP BY tool_name ORDER BY 2 DESC LIMIT 30`)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer rows.Close()
	type t struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	out := []t{}
	for rows.Next() {
		var x t
		rows.Scan(&x.Name, &x.Count)
		out = append(out, x)
	}
	writeJSON(w, 200, out)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := intParam(q, "limit", 50)
	offset := intParam(q, "offset", 0)
	project := q.Get("project")
	since := q.Get("since")
	textQ := q.Get("q")

	var conds []string
	var args []any
	if project != "" {
		conds = append(conds, "(s.project_path LIKE ? OR s.cwd LIKE ?)")
		args = append(args, "%"+project+"%", "%"+project+"%")
	}
	if since != "" {
		conds = append(conds, "s.started_at >= ?")
		args = append(args, since)
	}
	if textQ != "" {
		conds = append(conds, `(s.id IN (SELECT session_id FROM messages_fts WHERE messages_fts MATCH ?) OR s.project_path LIKE ? OR s.cwd LIKE ?)`)
		args = append(args, ftsEscape(textQ), "%"+textQ+"%", "%"+textQ+"%")
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	countSQL := `SELECT COUNT(*) FROM sessions s` + where
	var total int
	s.DB.QueryRow(countSQL, args...).Scan(&total)

	sql := `SELECT s.id, s.project_path, s.cwd, s.started_at, s.ended_at, COALESCE(s.title,''),
		(SELECT COUNT(*) FROM messages m WHERE m.session_id = s.id) AS msg_count,
		COALESCE((SELECT MAX(last_synced_at) FROM sync_state ss WHERE ss.file_path LIKE '%' || s.id || '%'), '') AS last_synced
		FROM sessions s` + where + ` ORDER BY s.started_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.DB.Query(sql, args...)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer rows.Close()
	type sess struct {
		ID         string `json:"id"`
		Project    string `json:"project"`
		CWD        string `json:"cwd"`
		StartedAt  string `json:"started_at"`
		EndedAt    string `json:"ended_at"`
		Title      string `json:"title"`
		Messages   int    `json:"messages"`
		LastSynced string `json:"last_synced_at"`
	}
	out := []sess{}
	for rows.Next() {
		var x sess
		var proj, cwd, ended *string
		rows.Scan(&x.ID, &proj, &cwd, &x.StartedAt, &ended, &x.Title, &x.Messages, &x.LastSynced)
		if proj != nil {
			x.Project = *proj
		}
		if cwd != nil {
			x.CWD = *cwd
		}
		if ended != nil {
			x.EndedAt = *ended
		}
		out = append(out, x)
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": out, "limit": limit, "offset": offset})
}

func (s *Server) sessionDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, 400, fmt.Errorf("missing id"))
		return
	}
	var sess struct {
		ID        string `json:"id"`
		Project   string `json:"project"`
		CWD       string `json:"cwd"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at"`
		Title     string `json:"title"`
	}
	var proj, cwd, ended *string
	err := s.DB.QueryRow(`SELECT id, project_path, cwd, started_at, ended_at, COALESCE(title,'') FROM sessions WHERE id=?`, id).
		Scan(&sess.ID, &proj, &cwd, &sess.StartedAt, &ended, &sess.Title)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	if proj != nil {
		sess.Project = *proj
	}
	if cwd != nil {
		sess.CWD = *cwd
	}
	if ended != nil {
		sess.EndedAt = *ended
	}

	rows, err := s.DB.Query(`SELECT uuid, role, kind, ts, COALESCE(text,''), COALESCE(tool_name,''), COALESCE(tool_input,''), COALESCE(tool_output,'')
		FROM messages WHERE session_id=? ORDER BY ts`, id)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer rows.Close()
	type msg struct {
		UUID       string `json:"uuid"`
		Role       string `json:"role"`
		Kind       string `json:"kind"`
		TS         string `json:"ts"`
		Text       string `json:"text"`
		ToolName   string `json:"tool_name,omitempty"`
		ToolInput  string `json:"tool_input,omitempty"`
		ToolOutput string `json:"tool_output,omitempty"`
	}
	msgs := []msg{}
	for rows.Next() {
		var m msg
		rows.Scan(&m.UUID, &m.Role, &m.Kind, &m.TS, &m.Text, &m.ToolName, &m.ToolInput, &m.ToolOutput)
		msgs = append(msgs, m)
	}
	writeJSON(w, 200, map[string]any{"session": sess, "messages": msgs})
}

func (s *Server) reindexSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, 400, fmt.Errorf("missing id"))
		return
	}

	files, err := jsonlFilesForSession(s.ProjectsDir, id)
	if err != nil || len(files) == 0 {
		writeErr(w, 404, fmt.Errorf("no JSONL file found for session %s", id))
		return
	}

	job := s.Jobs.Start("reindex", func(ctx context.Context, set func(string, float64), done func(any, error)) {
		set("clearing existing data", 0.05)
		if err := deleteSession(s.DB, id); err != nil {
			done(nil, err)
			return
		}
		for _, f := range files {
			if _, err := s.DB.Exec(`DELETE FROM sync_state WHERE file_path = ?`, f); err != nil {
				done(nil, err)
				return
			}
		}
		set("re-importing", 0.3)
		st := &ingest.ImportStats{}
		for i, f := range files {
			projPath := ingest.DecodeProjectPath(filepath.Base(filepath.Dir(f)))
			if err := importOneFileForReindex(ctx, s.DB, f, projPath, ingest.ToolSmall, st); err != nil {
				done(nil, err)
				return
			}
			set(fmt.Sprintf("imported %d/%d files", i+1, len(files)), 0.3+0.4*float64(i+1)/float64(len(files)))
		}
		if emb := s.embedder(); emb != nil {
			set("embedding", 0.75)
			n, err := ingest.EmbedPending(ctx, s.DB, emb, 0)
			if err != nil {
				done(nil, err)
				return
			}
			set(fmt.Sprintf("embedded %d chunks", n), 0.98)
		}
		done(st, nil)
	})
	writeJSON(w, 202, job)
}

// importOneFileForReindex skips offset tracking (we already deleted the row).
func importOneFileForReindex(ctx context.Context, db *store.DB, path, projPath string, mode ingest.ToolMode, st *ingest.ImportStats) error {
	// Reuse public RunImport with --project filter on the parent dir,
	// but since RunImport walks the whole tree this is simpler:
	// just call ingest's exposed RunImport with OnlyProject set.
	parent := filepath.Base(filepath.Dir(path))
	stats, err := ingest.RunImport(ctx, db, ingest.ImportOpts{
		ProjectsDir: filepath.Dir(filepath.Dir(path)),
		ToolMode:    mode,
		OnlyProject: parent,
	})
	if err != nil {
		return err
	}
	st.FilesScanned += stats.FilesScanned
	st.NewMessages += stats.NewMessages
	st.NewChunks += stats.NewChunks
	st.Errors += stats.Errors
	st.Skipped += stats.Skipped
	_ = projPath
	return nil
}

func deleteSession(db *store.DB, sessionID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM vec_chunks WHERE rowid IN (SELECT id FROM chunks WHERE session_id=?)`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM chunks WHERE session_id=?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages_fts WHERE session_id=?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id=?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id=?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func jsonlFilesForSession(projectsDir, sessionID string) ([]string, error) {
	var out []string
	err := filepath.Walk(projectsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		if strings.Contains(p, sessionID) {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeErr(w, 400, fmt.Errorf("missing q"))
		return
	}
	opts := search.Opts{
		Query:   query,
		Limit:   intParam(q, "limit", 20),
		Project: q.Get("project"),
		Role:    q.Get("role"),
	}
	if since := q.Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			opts.Since = t
		} else if t, err := time.Parse("2006-01-02", since); err == nil {
			opts.Since = t
		}
	}
	if q.Get("vector") != "false" {
		if emb := s.embedder(); emb != nil {
			opts.Embedder = emb
			opts.UseVector = true
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	hits, err := search.Run(ctx, s.DB, opts)
	if err != nil {
		writeErr(w, 500, err)
		return
	}

	// Project hits: rank session_paths that match the query string.
	type projHit struct {
		Path     string `json:"path"`
		Sessions int    `json:"sessions"`
	}
	projHits := []projHit{}
	pr, _ := s.DB.Query(`SELECT COALESCE(project_path, cwd, '(unknown)') AS p, COUNT(*) FROM sessions
		WHERE project_path LIKE ? OR cwd LIKE ?
		GROUP BY p ORDER BY 2 DESC LIMIT 8`, "%"+query+"%", "%"+query+"%")
	if pr != nil {
		for pr.Next() {
			var x projHit
			pr.Scan(&x.Path, &x.Sessions)
			projHits = append(projHits, x)
		}
		pr.Close()
	}

	writeJSON(w, 200, map[string]any{"hits": hits, "vector": opts.UseVector, "projects": projHits})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := ""
	if s.apiKey != "" {
		key = maskKey(s.apiKey)
	}
	writeJSON(w, 200, map[string]any{
		"openai_key_masked": key,
		"projects_dir":      s.ProjectsDir,
		"embed_model":       s.embedModel,
		"embed_dim":         s.embedDim,
		"base_url":          s.baseURL,
		"db_dim":            s.DB.Dim,
	})
}

func maskKey(k string) string {
	if len(k) < 12 {
		return "****"
	}
	return k[:6] + "..." + k[len(k)-4:]
}

func (s *Server) postConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OpenAIKey   string `json:"openai_api_key"`
		ProjectsDir string `json:"projects_dir"`
		EmbedModel  string `json:"embed_model"`
		EmbedDim    int    `json:"embed_dim"`
		BaseURL     string `json:"base_url"`
		ResetVec    bool   `json:"reset_vectors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.mu.Lock()
	if body.OpenAIKey != "" {
		s.apiKey = body.OpenAIKey
	}
	if body.ProjectsDir != "" {
		s.ProjectsDir = body.ProjectsDir
	}
	if body.EmbedModel != "" {
		s.embedModel = body.EmbedModel
	}
	if body.BaseURL != "" {
		// allow explicit reset to empty by sending "openai" sentinel
		if body.BaseURL == "openai" {
			s.baseURL = ""
		} else {
			s.baseURL = body.BaseURL
		}
	}
	newDim := s.embedDim
	if body.EmbedDim > 0 {
		newDim = body.EmbedDim
	}
	var resetErr error
	if newDim != s.embedDim || body.ResetVec {
		if body.ResetVec || newDim != s.DB.Dim {
			resetErr = s.DB.ResetEmbeddings(newDim)
			if resetErr == nil {
				s.embedDim = newDim
			}
		} else {
			s.embedDim = newDim
		}
	}
	s.rebuildEmbedder()
	s.mu.Unlock()
	if resetErr != nil {
		writeErr(w, 500, resetErr)
		return
	}
	s.getConfig(w, r)
}

func (s *Server) startImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path     string `json:"path"`
		ToolMode string `json:"tool_mode"`
		Embed    bool   `json:"embed"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Path == "" {
		body.Path = s.ProjectsDir
	}
	mode := ingest.ToolSmall
	switch strings.ToLower(body.ToolMode) {
	case "none":
		mode = ingest.ToolNone
	case "all":
		mode = ingest.ToolAll
	}
	job := s.Jobs.Start("import", func(ctx context.Context, set func(string, float64), done func(any, error)) {
		set("scanning", 0.05)
		stats, err := ingest.RunImport(ctx, s.DB, ingest.ImportOpts{
			ProjectsDir: body.Path,
			ToolMode:    mode,
		})
		if err != nil {
			done(stats, err)
			return
		}
		set(fmt.Sprintf("imported %d files (+%d msgs, +%d chunks)", stats.FilesScanned, stats.NewMessages, stats.NewChunks), 0.6)
		if body.Embed {
			if emb := s.embedder(); emb != nil {
				n, eerr := ingest.EmbedPending(ctx, s.DB, emb, 0)
				if eerr != nil {
					done(stats, eerr)
					return
				}
				set(fmt.Sprintf("embedded %d chunks", n), 0.98)
			} else {
				set("skipped embedding (no API key)", 0.95)
			}
		}
		done(stats, nil)
	})
	writeJSON(w, 202, job)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Jobs.List())
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j := s.Jobs.Get(r.PathValue("id"))
	if j == nil {
		writeErr(w, 404, fmt.Errorf("job not found"))
		return
	}
	writeJSON(w, 200, j)
}

func intParam(q map[string][]string, key string, def int) int {
	if v := getOne(q, key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getOne(q map[string][]string, key string) string {
	if v, ok := q[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

func ftsEscape(s string) string {
	fields := strings.Fields(s)
	for i, f := range fields {
		f = strings.ReplaceAll(f, `"`, `""`)
		fields[i] = `"` + f + `"`
	}
	return strings.Join(fields, " ")
}
