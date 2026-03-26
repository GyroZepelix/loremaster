package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest tracks which profile owns which synced skills.
type Manifest struct {
	Version  int                 `yaml:"version"`
	Profiles map[string][]string `yaml:"profiles"`
}

// New returns a Manifest with Version 1 and an initialized empty Profiles map.
func New() *Manifest {
	return &Manifest{
		Version:  1,
		Profiles: make(map[string][]string),
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

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		// Corrupted YAML — treat as absent.
		return nil, nil
	}

	if m.Profiles == nil {
		m.Profiles = make(map[string][]string)
	}
	return &m, nil
}

// Save writes a manifest to the given path atomically via temp file + os.Rename.
func Save(path string, m *Manifest) error {
	data, err := yaml.Marshal(m)
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
	return nil
}

// Exists reports whether a manifest file exists at the given path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SetProfile stores entries under the given profile name, replacing any existing entries.
func (m *Manifest) SetProfile(name string, entries []string) {
	m.Profiles[name] = entries
}

// GetProfile returns the entries for the given profile. The second return value
// is false if the profile does not exist.
func (m *Manifest) GetProfile(name string) ([]string, bool) {
	entries, ok := m.Profiles[name]
	return entries, ok
}

// RemoveProfile deletes the given profile from the manifest.
func (m *Manifest) RemoveProfile(name string) {
	delete(m.Profiles, name)
}

// ProfileNames returns a sorted slice of all profile names.
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
