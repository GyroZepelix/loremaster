package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgjalic/loremaster/internal/config"
	"github.com/dgjalic/loremaster/internal/git"
	"github.com/dgjalic/loremaster/internal/provider"
	loresync "github.com/dgjalic/loremaster/internal/sync"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync skills from configured sources",
	RunE:  runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	configPath, err := config.Locate(cwd)
	if err != nil {
		return fmt.Errorf("no lore.yml found (run 'lore init' first)")
	}

	f, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	cfg, err := config.Parse(f)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Resolve project root from config path
	projectRoot := resolveProjectRoot(configPath)

	prov, err := provider.Get(cfg.Provider)
	if err != nil {
		return err
	}

	syncer := &loresync.Syncer{
		GitFetcher:  &git.ExecGitFetcher{},
		Provider:    prov,
		ProjectRoot: projectRoot,
	}

	result, err := syncer.Sync(cfg)
	if err != nil {
		if result != nil {
			for _, e := range result.Errors {
				fmt.Fprintln(os.Stderr, e)
			}
			return fmt.Errorf("synced %d skills from %d sources with errors", result.Synced, result.Sources)
		}
		return err
	}

	fmt.Printf("Synced %d skills from %d sources\n", result.Synced, result.Sources)
	return nil
}

func resolveProjectRoot(configPath string) string {
	dir := filepath.Dir(configPath)
	base := filepath.Base(dir)

	// If config is inside .claude/ or .opencode/, project root is one level up
	if base == ".claude" || base == ".opencode" {
		return filepath.Dir(dir)
	}

	return dir
}
