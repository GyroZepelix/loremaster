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
	mf, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if manifest.Exists(manifestPath) && mf == nil {
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

	// Always create manifest
	if mf == nil {
		mf = manifest.New()
		// Retroactively register default profile's existing entries
		if err := retroRegisterDefault(mf, cfg, projectRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warning: retroactive default profile registration: %s\n", err)
		}
	}

	// Snapshot manifest entries before provider loop (explicit copy to prevent aliasing)
	existingEntries, _ := mf.GetProfile(profileName)
	snapshot := append([]string(nil), existingEntries...)

	// Phase 2: sync per provider
	totalSources := len(cfg.Skills)
	var totalSynced int
	var allErrors []string
	var allSyncedEntries []string
	anyProviderSucceeded := false

	for _, provName := range cfg.Providers {
		prov, err := provider.Get(provName)
		if err != nil {
			return fmt.Errorf("get provider %q: %w", provName, err)
		}

		syncer := &loresync.Syncer{
			GitFetcher:       fetcher,
			Provider:         prov,
			ProjectRoot:      projectRoot,
			ProfileName:      profileName,
			ManifestSnapshot: snapshot,
		}

		result, err := syncer.Sync(cfg, baseDirs)
		if err != nil {
			if result != nil {
				allErrors = append(allErrors, result.Errors...)
				totalSynced += result.Synced
				// Always collect entries even on error (partial success)
				allSyncedEntries = append(allSyncedEntries, result.Entries...)
				if len(result.Entries) > 0 {
					anyProviderSucceeded = true
				}
			} else {
				// Fatal error with no result — collect error and continue
				// to allow other providers to sync and manifest to be saved
				allErrors = append(allErrors, fmt.Sprintf("error: provider %q: %s", provName, err))
			}
			continue
		}

		totalSynced += result.Synced
		allErrors = append(allErrors, result.Errors...)
		allSyncedEntries = append(allSyncedEntries, result.Entries...)
		anyProviderSucceeded = true
	}

	// Provider removal cleanup and manifest update (only when at least one provider succeeded)
	if anyProviderSucceeded {
		removedEntries, cleanupErrs := cleanRemovedProviders(snapshot, cfg.Providers, projectRoot, gitignorePath)
		for _, e := range cleanupErrs {
			fmt.Fprintln(os.Stderr, e)
		}
		_ = removedEntries

		mf.SetProfile(profileName, allSyncedEntries)
	}

	// Always save manifest
	if err := manifest.Save(manifestPath, mf); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	// Ensure manifest in gitignore (idempotent)
	if err := gitignore.EnsureEntries(gitignorePath, []string{".lore-manifest.yml"}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %s\n", err)
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
		skillsParentDir := prov.SkillRoot(projectRoot)
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

// cleanRemovedProviders detects and cleans up skills from providers that were
// removed from the config. It compares snapshot entries against configured
// provider prefixes and removes any that no longer match.
func cleanRemovedProviders(snapshot []string, configProviders config.ProviderList, projectRoot, gitignorePath string) (removed []string, errs []string) {
	// Build configured prefix set
	configuredPrefixes := make(map[string]bool)
	for _, provName := range configProviders {
		prov, err := provider.Get(provName)
		if err != nil {
			errs = append(errs, fmt.Sprintf("warning: could not resolve provider %q: %s", provName, err))
			continue
		}
		prefix, err := skillRootPrefix(projectRoot, prov)
		if err != nil {
			errs = append(errs, fmt.Sprintf("warning: could not resolve provider %q skill root: %s", provName, err))
			continue
		}
		configuredPrefixes[prefix+"/"] = true
	}

	if len(configuredPrefixes) == 0 {
		return nil, errs
	}

	for _, entry := range snapshot {
		// Check if entry has a prefix matching any configured provider
		hasConfiguredProvider := false
		entrySlash := filepath.ToSlash(entry)
		for prefix := range configuredPrefixes {
			if strings.HasPrefix(entrySlash, prefix) {
				hasConfiguredProvider = true
				break
			}
		}
		if hasConfiguredProvider {
			continue // Still configured — skip
		}

		// Stale: from a removed provider
		absPath := filepath.Clean(filepath.Join(projectRoot, entry))

		// Validate path is within project root
		if !strings.HasPrefix(absPath, projectRoot+string(os.PathSeparator)) {
			errs = append(errs, fmt.Sprintf("warning: skipping %q: resolves outside project root", entry))
			continue
		}

		info, err := os.Lstat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Already gone, still collect for gitignore cleanup
				removed = append(removed, entry)
				continue
			}
			errs = append(errs, fmt.Sprintf("warning: could not stat %q: %s", entry, err))
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(absPath); err != nil {
				errs = append(errs, fmt.Sprintf("warning: could not remove symlink %q: %s", entry, err))
				continue
			}
			removed = append(removed, entry)
		} else if info.IsDir() {
			checksumFile := filepath.Join(absPath, ".lore-checksum")
			storedChecksum, readErr := os.ReadFile(checksumFile)
			if readErr != nil {
				errs = append(errs, fmt.Sprintf("warning: skipping %q: not managed by loremaster (no .lore-checksum)", entry))
				continue
			}
			currentChecksum, err := loresync.ComputeDirChecksum(absPath)
			if err != nil {
				errs = append(errs, fmt.Sprintf("warning: skipping %q: could not verify checksum: %s", entry, err))
				continue
			}
			if strings.TrimSpace(string(storedChecksum)) != currentChecksum {
				errs = append(errs, fmt.Sprintf("warning: skipping %q: local modifications detected", entry))
				continue
			}
			if err := os.RemoveAll(absPath); err != nil {
				errs = append(errs, fmt.Sprintf("warning: could not remove %q: %s", entry, err))
				continue
			}
			removed = append(removed, entry)
		} else {
			errs = append(errs, fmt.Sprintf("warning: skipping %q: unexpected file type", entry))
			continue
		}

		// Clean up empty parent directories, stopping at the actual skills root
		// (e.g., .pi/agent/skills) to avoid removing provider directories.
		cleanEmptyParents(filepath.Dir(absPath), skillRootForEntry(projectRoot, entry))
	}

	// Remove gitignore entries for cleaned-up skills
	if len(removed) > 0 {
		if err := gitignore.RemoveEntries(gitignorePath, removed); err != nil {
			errs = append(errs, fmt.Sprintf("warning: could not update .gitignore: %s", err))
		}
	}

	return removed, errs
}

func skillRootPrefix(projectRoot string, prov provider.Provider) (string, error) {
	rel, err := filepath.Rel(projectRoot, prov.SkillRoot(projectRoot))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func skillRootForEntry(projectRoot, entry string) string {
	entrySlash := filepath.ToSlash(filepath.Clean(entry))
	for _, prov := range provider.All() {
		prefix, err := skillRootPrefix(projectRoot, prov)
		if err != nil {
			continue
		}
		if entrySlash == prefix || strings.HasPrefix(entrySlash, prefix+"/") {
			return filepath.Join(projectRoot, filepath.FromSlash(prefix))
		}
	}

	parts := strings.Split(entrySlash, "/")
	if len(parts) >= 2 {
		return filepath.Join(projectRoot, filepath.FromSlash(strings.Join(parts[:2], "/")))
	}
	return projectRoot
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
	for _, configDir := range provider.ConfigDirs() {
		if root, ok := projectRootForConfigDir(dir, configDir); ok {
			return root
		}
	}

	return dir
}

func projectRootForConfigDir(dir, configDir string) (string, bool) {
	dir = filepath.Clean(dir)
	configDir = filepath.Clean(configDir)
	root := dir
	for range strings.Split(configDir, string(os.PathSeparator)) {
		root = filepath.Dir(root)
	}
	if filepath.Clean(filepath.Join(root, configDir)) == dir {
		return root, true
	}
	return "", false
}
