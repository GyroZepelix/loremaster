package sync

import (
	"fmt"
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
)

type Syncer struct {
	GitFetcher       git.Fetcher
	Provider         provider.Provider
	ProjectRoot      string
	ProfileName      string
	Manifest         *manifest.Manifest
	ManifestSnapshot []string
	Transactional    bool
}

type SyncResult struct {
	Synced  int
	Errors  []string
	Entries []string
	Items   []manifest.Item
	Removed []string
	Changes []Change
}

func FetchSources(fetcher git.Fetcher, sources []config.SkillSource) (map[string]string, []string) {
	baseDirs := make(map[string]string)
	var errs []string
	seen := make(map[string]bool)
	for _, source := range sources {
		if seen[source.Source] {
			continue
		}
		seen[source.Source] = true
		if config.IsGitSource(source.Source) {
			repoDir, err := cache.RepoDir(source.Source)
			if err != nil {
				errs = append(errs, fmt.Sprintf("error: resolve cache for %q: %s", source.Source, err))
				continue
			}
			if err := fetcher.CloneOrPull(source.Source, repoDir); err != nil {
				errs = append(errs, fmt.Sprintf("error: fetch %q: %s (check URL and authentication)", source.Source, err))
				continue
			}
			if source.Ref != "" {
				if err := fetcher.Checkout(repoDir, source.Ref); err != nil {
					errs = append(errs, fmt.Sprintf("error: checkout ref %q for %q: %s", source.Ref, source.Source, err))
					continue
				}
			}
			baseDirs[source.Source] = repoDir
			continue
		}

		absSource, err := filepath.Abs(source.Source)
		if err != nil {
			errs = append(errs, fmt.Sprintf("error: resolve local path %q: %s", source.Source, err))
			continue
		}
		info, err := os.Stat(absSource)
		if err != nil || !info.IsDir() {
			errs = append(errs, fmt.Sprintf("error: local path %q does not exist or is not a directory", source.Source))
			continue
		}
		baseDirs[source.Source] = absSource
	}
	return baseDirs, errs
}

func (s *Syncer) Sync(cfg *config.Config, baseDirs map[string]string) (*SyncResult, error) {
	if s.Provider == nil {
		return nil, fmt.Errorf("provider must be set before calling Sync()")
	}
	if err := cache.EnsureDir(); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	resources, err := prepareResources(cfg)
	if err != nil {
		return nil, err
	}
	result := &SyncResult{}
	desired := make(map[string]bool)

	for _, resource := range resources {
		for _, source := range resource.Sources {
			for _, entry := range source.ParsedIncludes {
				dst := s.Provider.ResourceDir(s.ProjectRoot, resource.Name, entry.Dst)
				rel, relErr := filepath.Rel(s.ProjectRoot, dst)
				if relErr != nil {
					return nil, fmt.Errorf("resolve destination for %s/%s: %w", resource.Name, entry.Dst, relErr)
				}
				desired[filepath.ToSlash(filepath.Clean(rel))] = true
			}
		}
	}

	for _, resource := range resources {
		for _, source := range resource.Sources {
			baseDir, ok := baseDirs[source.Source]
			if !ok {
				for _, entry := range source.ParsedIncludes {
					s.recordItemError(resource.Name, source, entry, result, "source fetch or resolution failed")
				}
				continue
			}
			for _, entry := range source.ParsedIncludes {
				s.syncItem(resource.Name, source, entry, baseDir, result)
			}
		}
	}

	s.reconcileStale(desired, result)
	sort.Strings(result.Entries)
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].Path < result.Items[j].Path })
	sort.Strings(result.Removed)
	result.Synced = len(result.Items)

	if s.Manifest == nil {
		s.updateLegacyGitignore(result)
	}
	if len(result.Errors) > 0 {
		return result, fmt.Errorf("sync completed with %d error(s)", len(result.Errors))
	}
	return result, nil
}

func prepareResources(cfg *config.Config) ([]config.Resource, error) {
	resources := cfg.AllResources()
	prepared := make([]config.Resource, len(resources))
	var destinations []config.IncludeEntry
	for i, resource := range resources {
		prepared[i] = config.Resource{Name: resource.Name, Sources: make([]config.SkillSource, len(resource.Sources))}
		copy(prepared[i].Sources, resource.Sources)
		for j := range prepared[i].Sources {
			source := &prepared[i].Sources[j]
			if source.Type == "" {
				source.Type = "soft"
			}
			if len(source.ParsedIncludes) == 0 {
				for _, raw := range source.Include {
					entry, err := config.ParseIncludeEntry(raw)
					if err != nil {
						return nil, fmt.Errorf("resource %q: %w", resource.Name, err)
					}
					source.ParsedIncludes = append(source.ParsedIncludes, entry)
				}
			}
			for _, entry := range source.ParsedIncludes {
				destinations = append(destinations, config.IncludeEntry{Src: entry.Src, Dst: filepath.ToSlash(filepath.Join(resource.Name, entry.Dst))})
			}
		}
	}
	if err := config.ValidateOverlaps(destinations); err != nil {
		return nil, fmt.Errorf("cross-resource overlap: %w", err)
	}
	return prepared, nil
}

func (s *Syncer) syncItem(resource string, source config.SkillSource, entry config.IncludeEntry, baseDir string, result *SyncResult) {
	srcPath := filepath.Join(baseDir, entry.Src)
	info, err := os.Stat(srcPath)
	if err != nil {
		s.recordItemError(resource, source, entry, result, fmt.Sprintf("source path %q was not found", srcPath))
		return
	}
	if resource == "skills" && !info.IsDir() {
		s.recordItemError(resource, source, entry, result, fmt.Sprintf("expected skill directory at %q", srcPath))
		return
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		s.recordItemError(resource, source, entry, result, fmt.Sprintf("source path %q is not a regular file or directory", srcPath))
		return
	}

	dstPath := s.Provider.ResourceDir(s.ProjectRoot, resource, entry.Dst)
	relPath := s.relativeDestination(resource, entry.Dst)
	if err := ensureNoSymlinkedParents(s.ProjectRoot, dstPath); err != nil {
		s.recordItemError(resource, source, entry, result, err.Error())
		return
	}
	managed, ownershipErr := s.managedState(relPath)
	if ownershipErr != nil {
		s.recordItemError(resource, source, entry, result, ownershipErr.Error())
		return
	}
	var linked LinkResult
	if s.Transactional {
		linked, err = LinkItemTransactional(srcPath, dstPath, source.Type, managed)
	} else {
		linked, err = LinkItem(srcPath, dstPath, source.Type, managed)
	}
	if err != nil {
		s.recordItemError(resource, source, entry, result, err.Error())
		return
	}

	item := manifest.Item{
		Path:            relPath,
		Provider:        s.Provider.Name(),
		Resource:        resource,
		Mode:            linked.Mode,
		Kind:            linked.Kind,
		Checksum:        linked.Checksum,
		ChecksumVersion: linked.ChecksumVersion,
		Target:          linked.Target,
	}
	result.Entries = append(result.Entries, relPath)
	result.Items = append(result.Items, item)
	if linked.Change != nil {
		result.Changes = append(result.Changes, *linked.Change)
	}
}

func (s *Syncer) recordItemError(resource string, source config.SkillSource, entry config.IncludeEntry, result *SyncResult, detail string) {
	rel := s.relativeDestination(resource, entry.Dst)
	result.Errors = append(result.Errors, fmt.Sprintf("error: provider %q resource %q item %q from %q to %q: %s", s.Provider.Name(), resource, entry.Src, source.Source, rel, detail))
}

func (s *Syncer) relativeDestination(resource string, destination string) string {
	path := s.Provider.ResourceDir(s.ProjectRoot, resource, destination)
	rel, _ := filepath.Rel(s.ProjectRoot, path)
	return filepath.ToSlash(filepath.Clean(rel))
}

func (s *Syncer) managedState(path string) (*ManagedState, error) {
	profile := s.ProfileName
	if profile == "" {
		profile = "default"
	}
	if s.Manifest != nil {
		owners := s.Manifest.Owners(path)
		if len(owners) == 0 {
			return nil, nil
		}
		if len(owners) != 1 || owners[0] != profile {
			return nil, fmt.Errorf("destination is owned by profile(s) %q", strings.Join(owners, ", "))
		}
		_, item, _ := s.Manifest.Owner(path)
		return &ManagedState{Mode: item.Mode, Kind: item.Kind, Checksum: item.Checksum, ChecksumVersion: item.ChecksumVersion, Target: item.Target, Legacy: item.Legacy}, nil
	}
	for _, entry := range s.ManifestSnapshot {
		if filepath.Clean(entry) == filepath.Clean(path) {
			return &ManagedState{Legacy: true}, nil
		}
	}
	return nil, nil
}

func (s *Syncer) previousItems() []manifest.Item {
	profile := s.ProfileName
	if profile == "" {
		profile = "default"
	}
	if s.Manifest != nil {
		items, _ := s.Manifest.GetProfileItems(profile)
		return items
	}
	items := make([]manifest.Item, 0, len(s.ManifestSnapshot))
	for _, path := range s.ManifestSnapshot {
		items = append(items, manifest.Item{Path: path, Provider: s.Provider.Name(), Resource: "skills", Legacy: true})
	}
	return items
}

func (s *Syncer) reconcileStale(desired map[string]bool, result *SyncResult) {
	configRoot := s.Provider.ConfigRoot(s.ProjectRoot)
	profile := s.ProfileName
	if profile == "" {
		profile = "default"
	}
	for _, item := range s.previousItems() {
		if desired[filepath.ToSlash(filepath.Clean(filepath.FromSlash(item.Path)))] || !pathWithinRoot(s.ProjectRoot, configRoot, item.Path) {
			continue
		}
		shared := false
		if s.Manifest != nil {
			for _, owner := range s.Manifest.Owners(item.Path) {
				if owner != profile {
					shared = true
					break
				}
			}
		}
		if shared {
			result.Removed = append(result.Removed, item.Path)
			continue
		}
		if s.Transactional {
			change, err := StageRemoveManagedItem(s.ProjectRoot, item)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: preserving %q: %s\n", item.Path, err)
				continue
			}
			if change != nil {
				result.Changes = append(result.Changes, *change)
			}
		} else {
			if err := RemoveManagedItem(s.ProjectRoot, item); err != nil {
				fmt.Fprintf(os.Stderr, "warning: preserving %q: %s\n", item.Path, err)
				continue
			}
			cleanEmptyParents(filepath.Dir(filepath.Join(s.ProjectRoot, item.Path)), configRoot)
		}
		result.Removed = append(result.Removed, item.Path)
	}
}

func pathWithinRoot(projectRoot string, root string, relativePath string) bool {
	absolute := filepath.Clean(filepath.Join(projectRoot, filepath.FromSlash(relativePath)))
	rel, err := filepath.Rel(root, absolute)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func RemoveManagedItem(projectRoot string, item manifest.Item) error {
	_, err := removeManagedItem(projectRoot, item, false)
	return err
}

func StageRemoveManagedItem(projectRoot string, item manifest.Item) (*Change, error) {
	return removeManagedItem(projectRoot, item, true)
}

func removeManagedItem(projectRoot string, item manifest.Item, transactional bool) (*Change, error) {
	cleanedPath := filepath.Clean(filepath.FromSlash(item.Path))
	if item.Path == "" || filepath.IsAbs(cleanedPath) || cleanedPath == "." || cleanedPath == ".." || strings.HasPrefix(cleanedPath, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid managed path %q", item.Path)
	}
	prov, err := provider.Get(item.Provider)
	if err != nil {
		return nil, fmt.Errorf("invalid managed provider %q", item.Provider)
	}
	resource, err := config.ValidateResourceName(item.Resource)
	if err != nil {
		return nil, fmt.Errorf("invalid managed resource %q", item.Resource)
	}
	path := filepath.Clean(filepath.Join(projectRoot, cleanedPath))
	resourceRoot := filepath.Join(prov.ConfigRoot(projectRoot), filepath.FromSlash(resource))
	rel, err := filepath.Rel(resourceRoot, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("managed path %q is outside its provider resource root", item.Path)
	}
	if err := ensureNoSymlinkedParents(projectRoot, path); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	state := &ManagedState{Mode: item.Mode, Kind: item.Kind, Checksum: item.Checksum, ChecksumVersion: item.ChecksumVersion, Target: item.Target, Legacy: item.Legacy}
	if err := verifyManagedDestination(path, state); err != nil {
		return nil, err
	}
	if !transactional {
		return nil, os.RemoveAll(path)
	}
	backup, err := reserveSiblingPath(filepath.Dir(path), ".lore-remove-")
	if err != nil {
		return nil, err
	}
	if err := os.Rename(path, backup); err != nil {
		return nil, err
	}
	return &Change{Destination: path, Backup: backup, CleanupStop: prov.ConfigRoot(projectRoot)}, nil
}

func ensureNoSymlinkedParents(projectRoot string, destination string) error {
	parent := filepath.Dir(destination)
	rel, err := filepath.Rel(projectRoot, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("destination %q is outside project root", destination)
	}
	if rel == "." {
		return nil
	}
	current := filepath.Clean(projectRoot)
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect destination parent %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination parent %q is a symlink", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("destination parent %q is not a directory", current)
		}
	}
	return nil
}

func InspectLegacyItem(projectRoot string, item manifest.Item) (manifest.Item, error) {
	path := filepath.Join(projectRoot, item.Path)
	info, err := os.Lstat(path)
	if err != nil {
		return item, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		item.Mode = "soft"
		item.Target, err = os.Readlink(path)
		if err != nil {
			return item, err
		}
		targetInfo, err := os.Stat(path)
		if err != nil {
			return item, err
		}
		if targetInfo.IsDir() {
			item.Kind = "directory"
		} else if targetInfo.Mode().IsRegular() {
			item.Kind = "file"
		} else {
			return item, fmt.Errorf("symlink target is not a regular file or directory")
		}
		item.Legacy = false
		return item, nil
	}
	if !info.IsDir() {
		return item, fmt.Errorf("legacy entry is not a managed symlink or directory")
	}
	stored, err := os.ReadFile(filepath.Join(path, ".lore-checksum"))
	if err != nil {
		return item, fmt.Errorf("legacy directory has no checksum")
	}
	item.Mode = "hard"
	item.Kind = "directory"
	item.Checksum = strings.TrimSpace(string(stored))
	item.ChecksumVersion = 1
	item.Legacy = false
	legacyCurrent, err := computeLegacyDirChecksum(path)
	if err != nil {
		return item, err
	}
	if item.Checksum != legacyCurrent {
		return item, fmt.Errorf("legacy directory has local modifications")
	}
	ambiguous, err := legacyTreeHasUnverifiableEntries(path)
	if err != nil {
		return item, err
	}
	if ambiguous {
		return item, fmt.Errorf("legacy directory contains symlinks or empty directories that cannot be verified")
	}
	item.Checksum, err = ComputeDirChecksum(path)
	if err != nil {
		return item, err
	}
	item.ChecksumVersion = currentChecksumVersion
	return item, nil
}

func legacyTreeHasUnverifiableEntries(root string) (bool, error) {
	ambiguous := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			ambiguous = true
			return nil
		}
		if path != root && entry.IsDir() {
			children, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			if len(children) == 0 {
				ambiguous = true
			}
		}
		return nil
	})
	return ambiguous, err
}

func (s *Syncer) updateLegacyGitignore(result *SyncResult) {
	path := filepath.Join(s.ProjectRoot, ".gitignore")
	if len(result.Entries) > 0 {
		if err := gitignore.EnsureEntries(path, result.Entries); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("update gitignore: %s", err))
		}
	}
	if len(result.Removed) > 0 {
		if err := gitignore.RemoveEntries(path, result.Removed); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cleanup gitignore: %s", err))
		}
	}
}

func cleanEmptyParents(dir string, stopAt string) {
	dir = filepath.Clean(dir)
	stopAt = filepath.Clean(stopAt)
	for dir != stopAt && dir != filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
