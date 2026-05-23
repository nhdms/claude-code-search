package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nhduc/claude-search/internal/store"
	"github.com/spf13/cobra"
)

const DefaultEmbedModel = "text-embedding-3-small"
const DefaultEmbedDim = 1536

var (
	flagDBPath      string
	flagProjectsDir string
	flagEmbedModel  string
	flagEmbedDim    int
)

func Execute() error {
	root := &cobra.Command{
		Use:           "claude-search",
		Short:         "Search your Claude Code conversation history",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	home, _ := os.UserHomeDir()
	defaultDB := filepath.Join(home, ".local", "share", "claude-search", "index.db")
	defaultProjects := filepath.Join(home, ".claude", "projects")

	root.PersistentFlags().StringVar(&flagDBPath, "db", defaultDB, "SQLite database path")
	root.PersistentFlags().StringVar(&flagProjectsDir, "projects-dir", defaultProjects, "Claude projects dir")
	root.PersistentFlags().StringVar(&flagEmbedModel, "embed-model", DefaultEmbedModel, "OpenAI embedding model")
	root.PersistentFlags().IntVar(&flagEmbedDim, "embed-dim", DefaultEmbedDim, "Embedding dimension")

	root.AddCommand(newImportCmd())
	root.AddCommand(newWatchCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newStatsCmd())
	root.AddCommand(newEmbedCmd())
	root.AddCommand(newServeCmd())

	return root.Execute()
}

func openDB() (*store.DB, error) {
	if err := os.MkdirAll(filepath.Dir(flagDBPath), 0o755); err != nil {
		return nil, err
	}
	db, err := store.Open(flagDBPath, flagEmbedDim)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", flagDBPath, err)
	}
	return db, nil
}
