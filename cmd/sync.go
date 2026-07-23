package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	Short: "Sync resources from configured sources",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringVarP(&profileFlag, "profile", "p", "", "sync a named profile (reads lore-<profile>.yml)")
	syncCmd.Flags().BoolVar(&pruneFlag, "prune", false, "remove resources from orphaned profiles")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) (returnErr error) {
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
			fmt.Println("no manifest found - nothing to prune")
			return nil
		}
		if err := migrateLegacyEntries(mf, projectRoot); err != nil {
			return fmt.Errorf("migrate legacy manifest: %w", err)
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

	manifestExisted := manifest.Exists(manifestPath)
	mf, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if manifestExisted && mf == nil {
		fmt.Fprintln(os.Stderr, "warning: .lore-manifest.yml is corrupted - proceeding without it (re-sync profiles to rebuild)")
	}

	// Orphan warning if manifest exists
	if mf != nil {
		orphans := mf.FindOrphaned(projectRoot, config.LocateProfile)
		for _, name := range orphans {
			fmt.Fprintf(os.Stderr, "warning: profile %q has entries in manifest but no config file found\n", name)
		}
	}

	allSources := cfg.AllSources()
	fetcher := &git.ExecGitFetcher{}
	baseDirs, sourceUpdates, fetchErrs := loresync.FetchSourcesWithUpdates(fetcher, allSources)
	out := io.Writer(os.Stdout)
	if cmd != nil {
		out = cmd.OutOrStdout()
	}
	rendered := false
	defer func() {
		if returnErr != nil && !rendered && hasRepositoryActivity(sourceUpdates) {
			renderSyncSummary(out, sourceUpdates, nil)
		}
	}()
	for _, fetchErr := range fetchErrs {
		fmt.Fprintln(os.Stderr, fetchErr)
	}

	if mf == nil {
		mf = manifest.New()
		if !manifestExisted {
			if err := retroRegisterDefault(mf, cfg, projectRoot); err != nil {
				fmt.Fprintf(os.Stderr, "warning: retroactive default profile registration: %s\n", err)
			}
		}
	}
	if err := migrateLegacyEntries(mf, projectRoot); err != nil {
		return fmt.Errorf("migrate legacy manifest: %w", err)
	}

	existingItems, _ := mf.GetProfileItems(profileName)
	nextItems := make(map[string]manifest.Item, len(existingItems))
	for _, item := range existingItems {
		nextItems[item.Path] = item
	}

	var totalSynced int
	var allErrors []string
	var allChanges []loresync.Change
	var allItemChanges []loresync.ItemChange
	for _, provName := range cfg.Providers {
		prov, err := provider.Get(provName)
		if err != nil {
			return fmt.Errorf("get provider %q: %w", provName, err)
		}
		syncer := &loresync.Syncer{
			GitFetcher:    fetcher,
			Provider:      prov,
			ProjectRoot:   projectRoot,
			ProfileName:   profileName,
			Manifest:      mf,
			SourceUpdates: sourceUpdates,
			Transactional: true,
		}
		result, syncErr := syncer.Sync(cfg, baseDirs)
		if result == nil {
			allErrors = append(allErrors, fmt.Sprintf("error: provider %q: %s", provName, syncErr))
			continue
		}
		totalSynced += result.Synced
		allErrors = append(allErrors, result.Errors...)
		allChanges = append(allChanges, result.Changes...)
		allItemChanges = append(allItemChanges, result.ItemChanges...)
		for _, path := range result.Removed {
			delete(nextItems, path)
		}
		for _, item := range result.Items {
			nextItems[item.Path] = item
		}
	}

	cleanupErrors, cleanupChanges := reconcileRemovedProviderItems(nextItems, cfg.Providers, projectRoot, mf, profileName, true)
	allChanges = append(allChanges, cleanupChanges...)
	for _, change := range cleanupChanges {
		if path, relErr := filepath.Rel(projectRoot, change.Destination); relErr == nil {
			allItemChanges = append(allItemChanges, loresync.ItemChange{Status: loresync.ItemDeleted, Path: filepath.ToSlash(filepath.Clean(path))})
		}
	}
	for _, cleanupErr := range cleanupErrors {
		fmt.Fprintln(os.Stderr, cleanupErr)
	}

	items := make([]manifest.Item, 0, len(nextItems))
	for _, item := range nextItems {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	mf.SetProfileItems(profileName, items)

	if err := manifest.Save(manifestPath, mf); err != nil {
		rollbackErrors := loresync.RollbackChanges(allChanges)
		if len(rollbackErrors) > 0 {
			return fmt.Errorf("save manifest: %w; rollback errors: %v", err, rollbackErrors)
		}
		return fmt.Errorf("save manifest: %w", err)
	}
	for _, commitErr := range loresync.CommitChanges(allChanges) {
		fmt.Fprintf(os.Stderr, "warning: %s\n", commitErr)
	}
	if err := reconcileGitignore(mf, gitignorePath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %s\n", err)
	}

	for _, syncErr := range allErrors {
		fmt.Fprintln(os.Stderr, syncErr)
	}
	renderSyncSummary(out, sourceUpdates, allItemChanges)
	rendered = true
	totalSources := countDistinctSources(allSources)
	if len(allErrors) > 0 {
		return fmt.Errorf("synced %d items from %d sources with errors", totalSynced, totalSources)
	}
	return nil
}

func hasRepositoryActivity(updates map[string]git.RepositoryUpdate) bool {
	for _, update := range updates {
		if update.Status == git.UpdateCloned || update.Status == git.UpdateFastForwarded {
			return true
		}
	}
	return false
}

func renderSyncSummary(w io.Writer, updates map[string]git.RepositoryUpdate, changes []loresync.ItemChange) {
	repositories := make([]git.RepositoryUpdate, 0, len(updates))
	for source, update := range updates {
		if update.Status != git.UpdateCloned && update.Status != git.UpdateFastForwarded {
			continue
		}
		if update.Source == "" {
			update.Source = source
		}
		update.Source = sanitizedRepositorySource(update.Source)
		repositories = append(repositories, update)
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i].Source < repositories[j].Source })

	seen := make(map[string]bool)
	items := make([]loresync.ItemChange, 0, len(changes))
	for _, change := range changes {
		change.Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(change.Path)))
		key := string(change.Status) + "\x00" + change.Path
		if !seen[key] {
			seen[key] = true
			items = append(items, change)
		}
	}
	rank := map[loresync.ItemChangeStatus]int{loresync.ItemAdded: 0, loresync.ItemUpdated: 1, loresync.ItemDeleted: 2}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return rank[items[i].Status] < rank[items[j].Status]
	})

	if len(repositories) == 0 && len(items) == 0 {
		fmt.Fprintln(w, "No repository or synced item changes.")
		return
	}
	if len(repositories) > 0 {
		fmt.Fprintln(w, "Repositories:")
		for _, update := range repositories {
			switch update.Status {
			case git.UpdateCloned:
				fmt.Fprintf(w, "  cloned %s @ %s\n", update.Source, update.AfterCommit)
			case git.UpdateFastForwarded:
				fmt.Fprintf(w, "  fast-forwarded %s %s -> %s\n", update.Source, update.BeforeCommit, update.AfterCommit)
			}
		}
	}
	if len(items) > 0 {
		fmt.Fprintln(w, "Synced items:")
		for _, change := range items {
			fmt.Fprintf(w, "  %s %s\n", change.Status, change.Path)
		}
	}
}

func sanitizedRepositorySource(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host == "" || parsed.User == nil {
		return source
	}
	parsed.User = nil
	return parsed.String()
}

func countDistinctSources(sources []config.SkillSource) int {
	seen := make(map[string]bool)
	for _, source := range sources {
		seen[source.Source] = true
	}
	return len(seen)
}

func migrateLegacyEntries(mf *manifest.Manifest, projectRoot string) error {
	for _, profile := range mf.ProfileNames() {
		items, _ := mf.GetProfileItems(profile)
		migrated := make([]manifest.Item, 0, len(items))
		for _, item := range items {
			if !item.Legacy {
				migrated = append(migrated, item)
				continue
			}
			prov, ok := providerForLegacySkill(projectRoot, item.Path)
			if !ok {
				return fmt.Errorf("cannot determine provider for %q", item.Path)
			}
			item.Provider = prov.Name()
			item.Resource = "skills"
			inspected, err := loresync.InspectLegacyItem(projectRoot, item)
			if errors.Is(err, loresync.ErrLegacyItemAbsent) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect %q: %w", item.Path, err)
			}
			migrated = append(migrated, inspected)
		}
		mf.SetProfileItems(profile, migrated)
	}
	return nil
}

func providerForLegacySkill(projectRoot string, entry string) (provider.Provider, bool) {
	absolute := filepath.Clean(filepath.Join(projectRoot, entry))
	for _, prov := range provider.All() {
		root := prov.SkillRoot(projectRoot)
		rel, err := filepath.Rel(root, absolute)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return prov, true
		}
	}
	return nil, false
}

func reconcileRemovedProviderItems(items map[string]manifest.Item, configured config.ProviderList, projectRoot string, mf *manifest.Manifest, profileName string, transactional bool) ([]string, []loresync.Change) {
	configuredSet := make(map[string]bool, len(configured))
	for _, name := range configured {
		configuredSet[name] = true
	}
	var warnings []string
	var changes []loresync.Change
	for path, item := range items {
		providerName := item.Provider
		var prov provider.Provider
		if providerName != "" {
			prov, _ = provider.Get(providerName)
		} else if inferred, ok := providerForLegacySkill(projectRoot, path); ok {
			prov = inferred
			providerName = inferred.Name()
		}
		if configuredSet[providerName] {
			continue
		}
		if prov == nil {
			warnings = append(warnings, fmt.Sprintf("warning: preserving %q: cannot determine owning provider", path))
			continue
		}
		shared := false
		if mf != nil {
			for _, owner := range mf.Owners(path) {
				if owner != profileName {
					shared = true
					break
				}
			}
		}
		if shared {
			delete(items, path)
			continue
		}
		if transactional {
			change, err := loresync.StageRemoveManagedItem(projectRoot, item)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("warning: preserving %q from removed provider %q: %s", path, providerName, err))
				continue
			}
			if change != nil {
				changes = append(changes, *change)
			}
		} else {
			if err := loresync.RemoveManagedItem(projectRoot, item); err != nil {
				warnings = append(warnings, fmt.Sprintf("warning: preserving %q from removed provider %q: %s", path, providerName, err))
				continue
			}
			cleanEmptyParents(filepath.Dir(filepath.Join(projectRoot, path)), prov.ConfigRoot(projectRoot))
		}
		delete(items, path)
	}
	sort.Strings(warnings)
	return warnings, changes
}

func reconcileGitignore(mf *manifest.Manifest, gitignorePath string) error {
	entries := append(mf.AllPaths(), ".lore-manifest.yml")
	return gitignore.SetManagedEntries(gitignorePath, entries)
}

// retroRegisterDefault scans each provider's skill directory for existing managed
// entries and registers them under the "default" profile in the manifest.
func retroRegisterDefault(mf *manifest.Manifest, cfg *config.Config, projectRoot string) error {
	cacheDir, err := cache.Dir()
	if err != nil {
		return fmt.Errorf("resolve cache dir: %w", err)
	}

	var allItems []manifest.Item
	for _, provName := range cfg.Providers {
		prov, err := provider.Get(provName)
		if err != nil {
			return fmt.Errorf("get provider %q: %w", provName, err)
		}
		skillsParentDir := prov.SkillRoot(projectRoot)
		if _, err := os.Stat(skillsParentDir); os.IsNotExist(err) {
			continue
		}
		relDir, err := filepath.Rel(projectRoot, skillsParentDir)
		if err != nil {
			return fmt.Errorf("compute relative path for %q: %w", provName, err)
		}
		entries, err := manifest.ScanManagedEntries(skillsParentDir, cacheDir)
		if err != nil {
			return fmt.Errorf("scan entries for %q: %w", provName, err)
		}
		for _, entry := range entries {
			item := manifest.Item{Path: filepath.ToSlash(filepath.Join(relDir, entry)), Provider: provName, Resource: "skills", Legacy: true}
			item, err = loresync.InspectLegacyItem(projectRoot, item)
			if err != nil {
				continue
			}
			allItems = append(allItems, item)
		}
	}

	if len(allItems) > 0 {
		mf.SetProfileItems("default", allItems)
	}
	return nil
}

// pruneOrphaned removes items belonging to profiles whose config files no longer exist.
func pruneOrphaned(mf *manifest.Manifest, projectRoot, manifestPath, gitignorePath string) error {
	if err := migrateLegacyEntries(mf, projectRoot); err != nil {
		return fmt.Errorf("migrate legacy manifest: %w", err)
	}
	orphans := mf.FindOrphaned(projectRoot, config.LocateProfile)
	if len(orphans) == 0 {
		fmt.Println("no orphaned profiles found - nothing to prune")
		return nil
	}

	var totalRemoved, totalSkipped int
	var changes []loresync.Change
	for _, name := range orphans {
		items, ok := mf.GetProfileItems(name)
		if !ok {
			continue
		}
		var retained []manifest.Item
		for _, item := range items {
			shared := false
			for _, owner := range mf.Owners(item.Path) {
				if owner != name {
					shared = true
					break
				}
			}
			if shared {
				totalRemoved++
				continue
			}
			change, err := loresync.StageRemoveManagedItem(projectRoot, item)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: preserving %q: %s\n", item.Path, err)
				retained = append(retained, item)
				totalSkipped++
				continue
			}
			if change != nil {
				changes = append(changes, *change)
			}
			totalRemoved++
		}
		if len(retained) > 0 {
			mf.SetProfileItems(name, retained)
		} else {
			mf.RemoveProfile(name)
		}
	}

	if err := manifest.Save(manifestPath, mf); err != nil {
		rollbackErrors := loresync.RollbackChanges(changes)
		if len(rollbackErrors) > 0 {
			return fmt.Errorf("save manifest: %w; rollback errors: %v", err, rollbackErrors)
		}
		return fmt.Errorf("save manifest: %w", err)
	}
	for _, commitErr := range loresync.CommitChanges(changes) {
		fmt.Fprintf(os.Stderr, "warning: %s\n", commitErr)
	}
	if err := reconcileGitignore(mf, gitignorePath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %s\n", err)
	}

	msg := fmt.Sprintf("Pruned %d items from %d orphaned profiles", totalRemoved, len(orphans))
	if totalSkipped > 0 {
		msg += fmt.Sprintf(" (%d preserved)", totalSkipped)
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
