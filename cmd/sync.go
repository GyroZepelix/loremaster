package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GyroZepelix/loremaster/internal/cache"
	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/git"
	"github.com/GyroZepelix/loremaster/internal/gitignore"
	"github.com/GyroZepelix/loremaster/internal/manifest"
	"github.com/GyroZepelix/loremaster/internal/provider"
	loresync "github.com/GyroZepelix/loremaster/internal/sync"
	"github.com/spf13/cobra"
)

var (
	profileFlag string
	pruneFlag   bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync skills from configured sources",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringVarP(&profileFlag, "profile", "p", "", "sync a named profile (reads lore-<profile>.yml)")
	syncCmd.Flags().BoolVar(&pruneFlag, "prune", false, "remove skills from orphaned profiles")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// --prune: handle before config parsing (config may not exist for orphaned profiles)
	if pruneFlag {
		projectRoot := cwd
		if configPath, locErr := config.LocateProfile(cwd, profileFlag); locErr == nil {
			projectRoot = resolveProjectRoot(configPath)
		}
		manifestPath := filepath.Join(projectRoot, ".lore-manifest.yml")
		gitignorePath := filepath.Join(projectRoot, ".gitignore")

		mf, err := manifest.Load(manifestPath)
		if err != nil {
			return fmt.Errorf("load manifest: %w", err)
		}
		if mf == nil {
			fmt.Println("no manifest found — nothing to prune")
			return nil
		}
		return pruneOrphaned(mf, projectRoot, manifestPath, gitignorePath)
	}

	configPath, err := config.LocateProfile(cwd, profileFlag)
	if err != nil {
		profileName := profileFlag
		if profileName == "" {
			profileName = "default"
		}
		return fmt.Errorf("no config found for profile %q (run 'lore init' first)", profileName)
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

	projectRoot := resolveProjectRoot(configPath)
	manifestPath := filepath.Join(projectRoot, ".lore-manifest.yml")
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	profileName := profileFlag
	if profileName == "" {
		profileName = "default"
	}

	// Load manifest (returns nil, nil for missing/corrupt)
	manifestOnDisk := manifest.Exists(manifestPath)
	mf, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if manifestOnDisk && mf == nil {
		fmt.Fprintln(os.Stderr, "warning: .lore-manifest.yml is corrupted — proceeding without it (re-sync profiles to rebuild)")
	}

	// Orphan warning if manifest exists
	if mf != nil {
		orphans := mf.FindOrphaned(projectRoot, config.LocateProfile)
		for _, name := range orphans {
			fmt.Fprintf(os.Stderr, "warning: profile %q has entries in manifest but no config file found\n", name)
		}
	}

	// Phase 1: fetch sources once
	fetcher := &git.ExecGitFetcher{}
	baseDirs, fetchErrs := loresync.FetchSources(fetcher, cfg.Skills)
	for _, e := range fetchErrs {
		fmt.Fprintln(os.Stderr, e)
	}

	// Phase 2: sync per provider
	totalSources := len(cfg.Skills)
	var totalSynced int
	var allErrors []string
	var perProviderEntries []string

	for _, provName := range cfg.Providers {
		prov, err := provider.Get(provName)
		if err != nil {
			return fmt.Errorf("get provider %q: %w", provName, err)
		}

		syncer := &loresync.Syncer{
			GitFetcher:  fetcher,
			Provider:    prov,
			ProjectRoot: projectRoot,
			Manifest:    mf,
			ProfileName: profileName,
		}

		result, err := syncer.Sync(cfg, baseDirs)
		if err != nil {
			if result != nil {
				allErrors = append(allErrors, result.Errors...)
				totalSynced += result.Synced
				if result.Manifest != nil {
					mf = result.Manifest
				}
				continue
			}
			return err
		}

		totalSynced += result.Synced
		allErrors = append(allErrors, result.Errors...)
		if result.Manifest != nil {
			mf = result.Manifest
		}

		// Capture this provider's profile entries (Sync replaces via SetProfile,
		// so we collect per-provider and merge after the loop)
		if mf != nil {
			entries, _ := mf.GetProfile(profileName)
			perProviderEntries = append(perProviderEntries, entries...)
		}
	}

	// Merge all providers' entries into one profile
	if mf != nil && len(perProviderEntries) > 0 {
		mf.SetProfile(profileName, perProviderEntries)
	}

	// Manifest lazy creation and save
	needManifest := profileName != "default" || len(cfg.Providers) > 1

	if needManifest && mf == nil {
		mf = manifest.New()
		// Retroactively register default profile's existing entries
		if err := retroRegisterDefault(mf, cfg, projectRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warning: retroactive default profile registration: %s\n", err)
		}
	}

	if mf != nil {
		if err := manifest.Save(manifestPath, mf); err != nil {
			return fmt.Errorf("save manifest: %w", err)
		}
		if !manifestOnDisk {
			if err := gitignore.EnsureEntries(gitignorePath, []string{".lore-manifest.yml"}); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %s\n", err)
			}
		}
	}

	// Print errors and summary
	for _, e := range allErrors {
		fmt.Fprintln(os.Stderr, e)
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("synced %d skills from %d sources with errors", totalSynced, totalSources)
	}

	fmt.Printf("Synced %d skills from %d sources\n", totalSynced, totalSources)
	return nil
}

// retroRegisterDefault scans each provider's skill directory for existing managed
// entries and registers them under the "default" profile in the manifest.
func retroRegisterDefault(mf *manifest.Manifest, cfg *config.Config, projectRoot string) error {
	cacheDir, err := cache.Dir()
	if err != nil {
		return fmt.Errorf("resolve cache dir: %w", err)
	}

	var allEntries []string
	for _, provName := range cfg.Providers {
		prov, err := provider.Get(provName)
		if err != nil {
			return fmt.Errorf("get provider %q: %w", provName, err)
		}
		skillsParentDir := filepath.Dir(prov.SkillDir(projectRoot, "dummy"))
		relDir, err := filepath.Rel(projectRoot, skillsParentDir)
		if err != nil {
			return fmt.Errorf("compute relative path for %q: %w", provName, err)
		}
		entries, err := manifest.ScanManagedEntries(skillsParentDir, cacheDir)
		if err != nil {
			return fmt.Errorf("scan entries for %q: %w", provName, err)
		}
		for _, e := range entries {
			allEntries = append(allEntries, filepath.Join(relDir, e))
		}
	}

	if len(allEntries) > 0 {
		mf.SetProfile("default", allEntries)
	}
	return nil
}

// pruneOrphaned removes skills belonging to orphaned profiles (profiles whose
// config files no longer exist).
func pruneOrphaned(mf *manifest.Manifest, projectRoot, manifestPath, gitignorePath string) error {
	orphans := mf.FindOrphaned(projectRoot, config.LocateProfile)
	if len(orphans) == 0 {
		fmt.Println("no orphaned profiles found — nothing to prune")
		return nil
	}

	var totalRemoved, totalSkipped int
	var removedGitignoreEntries []string

	for _, name := range orphans {
		entries, ok := mf.GetProfile(name)
		if !ok {
			continue
		}

		for _, entry := range entries {
			absPath := filepath.Clean(filepath.Join(projectRoot, entry))

			// Validate path is within project root
			if !strings.HasPrefix(absPath, projectRoot+string(os.PathSeparator)) {
				fmt.Fprintf(os.Stderr, "warning: skipping %q: resolves outside project root\n", entry)
				totalSkipped++
				continue
			}

			info, err := os.Lstat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					removedGitignoreEntries = append(removedGitignoreEntries, entry)
					totalRemoved++
					continue
				}
				fmt.Fprintf(os.Stderr, "warning: could not stat %q: %s\n", entry, err)
				totalSkipped++
				continue
			}

			if info.Mode()&os.ModeSymlink != 0 {
				// Symlink — safe to remove
				if err := os.Remove(absPath); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not remove symlink %q: %s\n", entry, err)
					totalSkipped++
					continue
				}
				removedGitignoreEntries = append(removedGitignoreEntries, entry)
				totalRemoved++
			} else if info.IsDir() {
				// Hard copy — check checksum
				checksumFile := filepath.Join(absPath, ".lore-checksum")
				storedChecksum, readErr := os.ReadFile(checksumFile)
				if readErr != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %q: not managed by loremaster (no .lore-checksum)\n", entry)
					totalSkipped++
					continue
				}

				currentChecksum, err := loresync.ComputeDirChecksum(absPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %q: could not verify checksum: %s\n", entry, err)
					totalSkipped++
					continue
				}

				if strings.TrimSpace(string(storedChecksum)) != currentChecksum {
					fmt.Fprintf(os.Stderr, "warning: skipping %q: local modifications detected\n", entry)
					totalSkipped++
					continue
				}

				if err := os.RemoveAll(absPath); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not remove %q: %s\n", entry, err)
					totalSkipped++
					continue
				}
				removedGitignoreEntries = append(removedGitignoreEntries, entry)
				totalRemoved++
			} else {
				fmt.Fprintf(os.Stderr, "warning: skipping %q: unexpected file type\n", entry)
				totalSkipped++
			}

			// Clean up empty parent directories
			cleanEmptyParents(filepath.Dir(absPath), projectRoot)
		}

		mf.RemoveProfile(name)
	}

	// Update gitignore
	if len(removedGitignoreEntries) > 0 {
		if err := gitignore.RemoveEntries(gitignorePath, removedGitignoreEntries); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %s\n", err)
		}
	}

	// Save updated manifest
	if err := manifest.Save(manifestPath, mf); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	msg := fmt.Sprintf("Pruned %d skills from %d orphaned profiles", totalRemoved, len(orphans))
	if totalSkipped > 0 {
		msg += fmt.Sprintf(" (%d skipped)", totalSkipped)
	}
	fmt.Println(msg)
	return nil
}

// cleanEmptyParents removes empty directories from dir up to (but not including) stopAt.
func cleanEmptyParents(dir, stopAt string) {
	dir = filepath.Clean(dir)
	stopAt = filepath.Clean(stopAt)
	for dir != stopAt && dir != filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
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
