package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/provider"
	"gopkg.in/yaml.v3"
)

const CurrentVersion = 2

type Item struct {
	Path            string `yaml:"path"`
	Provider        string `yaml:"provider,omitempty"`
	Resource        string `yaml:"resource,omitempty"`
	Mode            string `yaml:"mode,omitempty"`
	Kind            string `yaml:"kind,omitempty"`
	Checksum        string `yaml:"checksum,omitempty"`
	ChecksumVersion int    `yaml:"checksum_version,omitempty"`
	Target          string `yaml:"target,omitempty"`
	Legacy          bool   `yaml:"-"`
}

type Manifest struct {
	Version      int
	Profiles     map[string][]string
	profileItems map[string]map[string]Item
}

type manifestV2 struct {
	Version  int               `yaml:"version"`
	Profiles map[string][]Item `yaml:"profiles"`
}

func New() *Manifest {
	return &Manifest{
		Version:      CurrentVersion,
		Profiles:     make(map[string][]string),
		profileItems: make(map[string]map[string]Item),
	}
}

// Load reads a manifest from the given path. It returns (nil, nil) if the file
// is missing or contains unparseable YAML.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var header struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return nil, nil
	}
	if header.Version == 0 {
		header.Version = 1
	}

	switch header.Version {
	case 1:
		var legacy struct {
			Profiles map[string][]string `yaml:"profiles"`
		}
		if err := yaml.Unmarshal(data, &legacy); err != nil {
			return nil, nil
		}
		m := New()
		for profile, paths := range legacy.Profiles {
			m.SetProfile(profile, paths)
		}
		return m, nil
	case CurrentVersion:
		var wire manifestV2
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&wire); err != nil {
			return nil, nil
		}
		m := New()
		for profile, items := range wire.Profiles {
			for _, item := range items {
				if err := validateItem(item); err != nil {
					return nil, nil
				}
			}
			m.SetProfileItems(profile, items)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported manifest version %d", header.Version)
	}
}

// Save writes a version 2 manifest atomically via temp file + os.Rename.
func Save(path string, m *Manifest) error {
	wire := manifestV2{Version: CurrentVersion, Profiles: make(map[string][]Item, len(m.Profiles))}
	for profile, paths := range m.Profiles {
		items := make([]Item, 0, len(paths))
		for _, path := range paths {
			item, ok := m.itemForProfile(profile, path)
			if !ok {
				item = Item{Path: path}
			}
			item.Path = path
			if err := validateItem(item); err != nil {
				return fmt.Errorf("invalid manifest item %q: %w", path, err)
			}
			items = append(items, item)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
		wire.Profiles[profile] = items
	}
	data, err := yaml.Marshal(wire)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".lore-manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming manifest: %w", err)
	}
	m.Version = CurrentVersion
	return nil
}

func validateItem(item Item) error {
	cleanedPath, err := config.ValidateResourceName(item.Path)
	if err != nil || cleanedPath != item.Path {
		return fmt.Errorf("invalid path")
	}
	if !provider.IsSupported(item.Provider) {
		return fmt.Errorf("invalid provider %q", item.Provider)
	}
	resource, err := config.ValidateResourceName(item.Resource)
	if err != nil || resource != item.Resource {
		return fmt.Errorf("invalid resource %q", item.Resource)
	}
	if item.Legacy {
		return fmt.Errorf("legacy item was not migrated")
	}
	if item.Mode != "soft" && item.Mode != "hard" {
		return fmt.Errorf("invalid mode %q", item.Mode)
	}
	if item.Kind != "file" && item.Kind != "directory" {
		return fmt.Errorf("invalid kind %q", item.Kind)
	}
	if item.Mode == "soft" && item.Target == "" {
		return fmt.Errorf("soft item has no recorded target")
	}
	if item.Mode == "hard" && item.Checksum == "" {
		return fmt.Errorf("hard item has no checksum")
	}
	if item.ChecksumVersion < 0 || item.ChecksumVersion > 2 || item.ChecksumVersion == 1 {
		return fmt.Errorf("invalid checksum version %d", item.ChecksumVersion)
	}
	return nil
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (m *Manifest) SetProfile(name string, paths []string) {
	if m.profileItems == nil {
		m.profileItems = make(map[string]map[string]Item)
	}
	m.Profiles[name] = append([]string(nil), paths...)
	items := make(map[string]Item, len(paths))
	for _, path := range paths {
		items[path] = Item{Path: path, Legacy: true}
	}
	m.profileItems[name] = items
}

func (m *Manifest) SetProfileItems(name string, items []Item) {
	if m.profileItems == nil {
		m.profileItems = make(map[string]map[string]Item)
	}
	paths := make([]string, 0, len(items))
	metadata := make(map[string]Item, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
		metadata[item.Path] = item
	}
	m.Profiles[name] = paths
	m.profileItems[name] = metadata
}

func (m *Manifest) GetProfile(name string) ([]string, bool) {
	entries, ok := m.Profiles[name]
	return entries, ok
}

func (m *Manifest) GetProfileItems(name string) ([]Item, bool) {
	paths, ok := m.Profiles[name]
	if !ok {
		return nil, false
	}
	items := make([]Item, 0, len(paths))
	for _, path := range paths {
		item, exists := m.itemForProfile(name, path)
		if !exists {
			item = Item{Path: path}
		}
		items = append(items, item)
	}
	return items, true
}

func (m *Manifest) itemForProfile(profile string, path string) (Item, bool) {
	items, ok := m.profileItems[profile]
	if !ok {
		return Item{}, false
	}
	item, ok := items[path]
	return item, ok
}

func (m *Manifest) Owner(path string) (string, Item, bool) {
	for _, profile := range m.ProfileNames() {
		for _, candidate := range m.Profiles[profile] {
			if candidate == path {
				item, ok := m.itemForProfile(profile, path)
				if !ok {
					item = Item{Path: path}
				}
				return profile, item, true
			}
		}
	}
	return "", Item{}, false
}

func (m *Manifest) Owners(path string) []string {
	var owners []string
	for _, profile := range m.ProfileNames() {
		for _, candidate := range m.Profiles[profile] {
			if candidate == path {
				owners = append(owners, profile)
				break
			}
		}
	}
	return owners
}

func (m *Manifest) AllPaths() []string {
	set := make(map[string]bool)
	for _, paths := range m.Profiles {
		for _, path := range paths {
			set[path] = true
		}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (m *Manifest) RemoveProfile(name string) {
	delete(m.Profiles, name)
	delete(m.profileItems, name)
}

func (m *Manifest) ProfileNames() []string {
	names := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ScanManagedEntries walks skillsParentDir to find entries managed by loremaster:
// symlinks pointing into cacheDir and directories containing .lore-checksum.
// Returns relative paths from skillsParentDir, sorted.
func ScanManagedEntries(skillsParentDir, cacheDir string) ([]string, error) {
	absCacheDir, err := filepath.Abs(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("resolving cache dir: %w", err)
	}

	var managed []string

	err = filepath.Walk(skillsParentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself.
		if path == skillsParentDir {
			return nil
		}

		// Use Lstat to detect symlinks (Walk follows them by default with the info it provides).
		linfo, lerr := os.Lstat(path)
		if lerr != nil {
			return lerr
		}

		rel, err := filepath.Rel(skillsParentDir, path)
		if err != nil {
			return err
		}

		// Check for symlink pointing into cache.
		if linfo.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			absTarget, err := filepath.Abs(target)
			if err != nil {
				return err
			}
			if strings.HasPrefix(absTarget, absCacheDir+string(filepath.Separator)) || absTarget == absCacheDir {
				managed = append(managed, rel)
			}
			// Walk doesn't follow symlinks, so they are non-directory entries.
			// Returning SkipDir here would skip the entire containing directory.
			return nil
		}

		// Check for directory with .lore-checksum.
		if linfo.IsDir() {
			checksumPath := filepath.Join(path, ".lore-checksum")
			if _, err := os.Stat(checksumPath); err == nil {
				managed = append(managed, rel)
				return filepath.SkipDir
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning managed entries: %w", err)
	}

	sort.Strings(managed)
	return managed, nil
}

// FindOrphaned returns a sorted slice of profile names whose config files
// don't exist. For each profile, locateFn is called with (dir, profileName);
// if it returns an error the profile is considered orphaned.
func (m *Manifest) FindOrphaned(dir string, locateFn func(dir, profile string) (string, error)) []string {
	var orphaned []string
	for name := range m.Profiles {
		if _, err := locateFn(dir, name); err != nil {
			orphaned = append(orphaned, name)
		}
	}
	sort.Strings(orphaned)
	return orphaned
}
