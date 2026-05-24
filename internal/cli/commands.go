package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"net/http"

	"github.com/fsnotify/fsnotify"
	"github.com/nhduc/claude-search/internal/api"
	"github.com/nhduc/claude-search/internal/ingest"
	"github.com/nhduc/claude-search/internal/search"
	"github.com/nhduc/claude-search/internal/store"
	"github.com/spf13/cobra"
)

func parseToolMode(s string) (ingest.ToolMode, error) {
	switch strings.ToLower(s) {
	case "none":
		return ingest.ToolNone, nil
	case "small":
		return ingest.ToolSmall, nil
	case "all":
		return ingest.ToolAll, nil
	}
	return 0, fmt.Errorf("invalid --embed-tool-output: %q (expect none|small|all)", s)
}

func newImportCmd() *cobra.Command {
	var toolMode string
	var only string
	var doEmbed bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Backfill conversation history into the index",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseToolMode(toolMode)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ctx, cancel := signalCtx()
			defer cancel()

			fmt.Printf("Importing from %s ...\n", flagProjectsDir)
			start := time.Now()
			stats, err := ingest.RunImport(ctx, db, ingest.ImportOpts{
				ProjectsDir: flagProjectsDir,
				ToolMode:    mode,
				OnlyProject: only,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Imported: files=%d new_messages=%d new_chunks=%d skipped=%d errors=%d in %s\n",
				stats.FilesScanned, stats.NewMessages, stats.NewChunks, stats.Skipped, stats.Errors,
				time.Since(start).Round(time.Millisecond))

			if doEmbed {
				return runEmbed(ctx, db)
			}
			pending, _ := ingest.PendingChunks(db)
			fmt.Printf("Pending chunks to embed: %d (run `claude-search embed` to vectorize)\n", pending)
			return nil
		},
	}
	cmd.Flags().StringVar(&toolMode, "embed-tool-output", "small", "Tool output handling: none|small|all")
	cmd.Flags().StringVar(&only, "project", "", "Only import this project dir name (under projects-dir)")
	cmd.Flags().BoolVar(&doEmbed, "embed", false, "Also run embedding after import")
	return cmd
}

func newEmbedCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Embed pending chunks using OpenAI",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ctx, cancel := signalCtx()
			defer cancel()
			_ = limit
			return runEmbed(ctx, db)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Max chunks to embed (0=all)")
	return cmd
}

func runEmbed(ctx context.Context, db *store.DB) error {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" && flagBaseURL == "" {
		pending, _ := ingest.PendingChunks(db)
		fmt.Printf("Neither OPENAI_API_KEY nor --base-url set; skipping embedding (%d pending)\n", pending)
		return nil
	}
	if key == "" {
		key = "local"
	}
	emb := ingest.NewEmbedderWithBase(key, flagEmbedModel, flagEmbedDim, flagBaseURL)
	pending, _ := ingest.PendingChunks(db)
	target := flagEmbedModel
	if flagBaseURL != "" {
		target += " @ " + flagBaseURL
	}
	fmt.Printf("Embedding %d chunks with %s...\n", pending, target)
	start := time.Now()
	n, err := ingest.EmbedPending(ctx, db, emb, 0)
	if err != nil {
		return err
	}
	fmt.Printf("Embedded %d chunks in %s\n", n, time.Since(start).Round(time.Millisecond))
	return nil
}

func newWatchCmd() *cobra.Command {
	var toolMode string
	var embedOnTick bool
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch projects dir and sync changes live",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseToolMode(toolMode)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ctx, cancel := signalCtx()
			defer cancel()

			fmt.Printf("Initial sync...\n")
			if _, err := ingest.RunImport(ctx, db, ingest.ImportOpts{
				ProjectsDir: flagProjectsDir, ToolMode: mode,
			}); err != nil {
				return err
			}

			w, err := fsnotify.NewWatcher()
			if err != nil {
				return err
			}
			defer w.Close()
			if err := w.Add(flagProjectsDir); err != nil {
				return err
			}
			entries, _ := os.ReadDir(flagProjectsDir)
			for _, e := range entries {
				if e.IsDir() {
					_ = w.Add(filepath.Join(flagProjectsDir, e.Name()))
				}
			}

			fmt.Printf("Watching %s (Ctrl-C to stop)\n", flagProjectsDir)
			debounce := time.NewTimer(time.Hour)
			debounce.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case ev := <-w.Events:
					if ev.Op&fsnotify.Create != 0 {
						info, err := os.Stat(ev.Name)
						if err == nil && info.IsDir() {
							_ = w.Add(ev.Name)
						}
					}
					debounce.Reset(2 * time.Second)
				case err := <-w.Errors:
					fmt.Fprintln(os.Stderr, "watcher error:", err)
				case <-debounce.C:
					stats, err := ingest.RunImport(ctx, db, ingest.ImportOpts{
						ProjectsDir: flagProjectsDir, ToolMode: mode,
					})
					if err != nil {
						fmt.Fprintln(os.Stderr, "sync error:", err)
						continue
					}
					if stats.NewMessages > 0 || stats.NewChunks > 0 {
						fmt.Printf("synced: +%d messages, +%d chunks\n", stats.NewMessages, stats.NewChunks)
						if embedOnTick {
							key := os.Getenv("OPENAI_API_KEY")
							if key != "" || flagBaseURL != "" {
								if key == "" {
									key = "local"
								}
								emb := ingest.NewEmbedderWithBase(key, flagEmbedModel, flagEmbedDim, flagBaseURL)
								n, err := ingest.EmbedPending(ctx, db, emb, 0)
								if err != nil {
									fmt.Fprintln(os.Stderr, "embed:", err)
								} else if n > 0 {
									fmt.Printf("embedded: +%d chunks\n", n)
								}
							}
						}
					}
				}
			}
		},
	}
	cmd.Flags().StringVar(&toolMode, "embed-tool-output", "small", "Tool output handling: none|small|all")
	cmd.Flags().BoolVar(&embedOnTick, "embed", true, "Embed new chunks on each sync tick")
	return cmd
}

func newSearchCmd() *cobra.Command {
	var limit int
	var project, role, since string
	var noVector bool
	cmd := &cobra.Command{
		Use:   "search [query...]",
		Short: "Search past conversations",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			ctx, cancel := signalCtx()
			defer cancel()

			opts := search.Opts{
				Query:   query,
				Limit:   limit,
				Project: project,
				Role:    role,
			}
			if since != "" {
				t, err := parseSince(since)
				if err != nil {
					return err
				}
				opts.Since = t
			}
			if !noVector {
				key := os.Getenv("OPENAI_API_KEY")
				if key != "" || flagBaseURL != "" {
					if key == "" {
						key = "local"
					}
					opts.Embedder = ingest.NewEmbedderWithBase(key, flagEmbedModel, flagEmbedDim, flagBaseURL)
					opts.UseVector = true
				} else {
					fmt.Fprintln(os.Stderr, "note: no OPENAI_API_KEY or --base-url; FTS-only search")
				}
			}

			hits, err := search.Run(ctx, db, opts)
			if err != nil {
				return err
			}
			if len(hits) == 0 {
				fmt.Println("(no results)")
				return nil
			}
			for i, h := range hits {
				fmt.Printf("\n[%d] score=%.4f  %s  %s\n", i+1, h.Score, h.Role, h.TS)
				fmt.Printf("    session: %s\n", h.SessionID)
				if h.Project != "" {
					fmt.Printf("    project: %s\n", h.Project)
				}
				snippet := h.Text
				if len(snippet) > 400 {
					snippet = snippet[:400] + "..."
				}
				for _, ln := range strings.Split(snippet, "\n") {
					fmt.Printf("    %s\n", ln)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")
	cmd.Flags().StringVar(&project, "project", "", "Filter by project path substring")
	cmd.Flags().StringVar(&role, "role", "", "Filter by role (user|assistant)")
	cmd.Flags().StringVar(&since, "since", "", "ISO date or duration (e.g. 7d, 24h, 2025-01-01)")
	cmd.Flags().BoolVar(&noVector, "no-vector", false, "Disable vector search (FTS only)")
	return cmd
}

func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <session_id>",
		Short: "Show messages in a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.Query(`SELECT ts, role, kind, COALESCE(text,''), COALESCE(tool_name,'')
				FROM messages WHERE session_id=? ORDER BY ts`, args[0])
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var ts, role, kind, text, tool string
				if err := rows.Scan(&ts, &role, &kind, &text, &tool); err != nil {
					return err
				}
				fmt.Printf("\n[%s] %s/%s", ts, role, kind)
				if tool != "" {
					fmt.Printf(" (%s)", tool)
				}
				fmt.Println()
				snippet := text
				if len(snippet) > 1000 {
					snippet = snippet[:1000] + "..."
				}
				fmt.Println(snippet)
			}
			return nil
		},
	}
	return cmd
}

func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show index statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			var sessions, messages, chunks, embedded int
			db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions)
			db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&messages)
			db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunks)
			db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE embedded=1`).Scan(&embedded)
			fmt.Printf("db:        %s\n", flagDBPath)
			fmt.Printf("sessions:  %d\n", sessions)
			fmt.Printf("messages:  %d\n", messages)
			fmt.Printf("chunks:    %d\n", chunks)
			fmt.Printf("embedded:  %d (%.1f%%)\n", embedded, pct(embedded, chunks))
			return nil
		},
	}
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100.0 * float64(a) / float64(b)
}

func parseSince(s string) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Now().AddDate(0, 0, -days), nil
		}
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid --since %q", s)
}

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run HTTP API server for the dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			srv := api.New(db, flagEmbedModel, flagEmbedDim, flagProjectsDir)
			fmt.Printf("Listening on %s (vector_ready=%v)\n", addr, os.Getenv("OPENAI_API_KEY") != "")
			s := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
			ctx, cancel := signalCtx()
			defer cancel()
			go func() {
				<-ctx.Done()
				_ = s.Shutdown(context.Background())
			}()
			err = s.ListenAndServe()
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7070", "Address to bind")
	return cmd
}

func signalCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}
