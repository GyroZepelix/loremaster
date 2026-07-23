package sync

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const currentChecksumVersion = 2

type ManagedState struct {
	Mode            string
	Kind            string
	Checksum        string
	ChecksumVersion int
	Target          string
	Legacy          bool
}

type LinkResult struct {
	Mode            string
	Kind            string
	Checksum        string
	ChecksumVersion int
	Target          string
	Change          *Change
}

type Change struct {
	Destination string
	Backup      string
	CleanupStop string
	Created     bool
}

func LinkItem(src string, dst string, linkType string, managed *ManagedState) (LinkResult, error) {
	return linkItem(src, dst, linkType, managed, false)
}

func LinkItemTransactional(src string, dst string, linkType string, managed *ManagedState) (LinkResult, error) {
	return linkItem(src, dst, linkType, managed, true)
}

func linkItem(src string, dst string, linkType string, managed *ManagedState, transactional bool) (LinkResult, error) {
	info, err := os.Stat(src)
	if err != nil {
		return LinkResult{}, fmt.Errorf("stat source %q: %w", src, err)
	}
	kind := ""
	switch {
	case info.IsDir():
		kind = "directory"
	case info.Mode().IsRegular():
		kind = "file"
	default:
		return LinkResult{}, fmt.Errorf("source %q is not a regular file or directory", src)
	}
	if linkType != "soft" && linkType != "hard" {
		return LinkResult{}, fmt.Errorf("invalid link type %q", linkType)
	}

	destinationExists := false
	if _, err := os.Lstat(dst); err == nil {
		destinationExists = true
		if managed == nil {
			return LinkResult{}, fmt.Errorf("destination %q exists but is not owned by loremaster", dst)
		}
		if err := verifyManagedDestination(dst, managed); err != nil {
			return LinkResult{}, err
		}
	} else if !os.IsNotExist(err) {
		return LinkResult{}, fmt.Errorf("stat destination %q: %w", dst, err)
	}

	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return LinkResult{}, fmt.Errorf("create parent dir for %q: %w", dst, err)
	}
	staged, err := reserveSiblingPath(parent, ".lore-stage-")
	if err != nil {
		return LinkResult{}, fmt.Errorf("reserve staged path for %q: %w", dst, err)
	}
	defer func() {
		if staged != "" {
			os.RemoveAll(staged)
		}
	}()

	result := LinkResult{Mode: linkType, Kind: kind}
	if linkType == "soft" {
		if err := os.Symlink(src, staged); err != nil {
			return LinkResult{}, fmt.Errorf("stage symlink for %q: %w", dst, err)
		}
		result.Target = src
	} else {
		if kind == "directory" {
			if err := copyDir(src, staged); err != nil {
				return LinkResult{}, fmt.Errorf("stage copy for %q: %w", dst, err)
			}
		} else if err := copyFile(src, staged); err != nil {
			return LinkResult{}, fmt.Errorf("stage copy for %q: %w", dst, err)
		}
		checksum, err := ComputePathChecksum(staged, kind)
		if err != nil {
			return LinkResult{}, fmt.Errorf("compute staged checksum for %q: %w", dst, err)
		}
		result.Checksum = checksum
		result.ChecksumVersion = currentChecksumVersion
	}

	if !destinationExists {
		if err := os.Rename(staged, dst); err != nil {
			return LinkResult{}, fmt.Errorf("install staged item at %q: %w", dst, err)
		}
		staged = ""
		if transactional {
			result.Change = &Change{Destination: dst, Created: true}
		}
		return result, nil
	}

	backup, err := reserveSiblingPath(parent, ".lore-backup-")
	if err != nil {
		return LinkResult{}, fmt.Errorf("reserve backup path for %q: %w", dst, err)
	}
	if err := os.Rename(dst, backup); err != nil {
		return LinkResult{}, fmt.Errorf("move existing %q to backup: %w", dst, err)
	}
	if err := os.Rename(staged, dst); err != nil {
		rollbackErr := os.Rename(backup, dst)
		if rollbackErr != nil {
			return LinkResult{}, fmt.Errorf("install staged item at %q: %v; rollback failed: %w", dst, err, rollbackErr)
		}
		return LinkResult{}, fmt.Errorf("install staged item at %q: %w", dst, err)
	}
	staged = ""
	if transactional {
		result.Change = &Change{Destination: dst, Backup: backup}
		return result, nil
	}
	if err := os.RemoveAll(backup); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove backup %q: %s\n", backup, err)
	}
	return result, nil
}

func CommitChanges(changes []Change) []error {
	var errs []error
	for _, change := range changes {
		if change.Backup == "" {
			continue
		}
		if err := os.RemoveAll(change.Backup); err != nil {
			errs = append(errs, fmt.Errorf("remove backup %q: %w", change.Backup, err))
			continue
		}
		if change.CleanupStop != "" {
			cleanEmptyParents(filepath.Dir(change.Destination), change.CleanupStop)
		}
	}
	return errs
}

func RollbackChanges(changes []Change) []error {
	var errs []error
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.Backup == "" {
			if change.Created {
				if err := os.RemoveAll(change.Destination); err != nil {
					errs = append(errs, fmt.Errorf("remove created item %q: %w", change.Destination, err))
				}
			}
			continue
		}
		if err := os.RemoveAll(change.Destination); err != nil {
			errs = append(errs, fmt.Errorf("remove replacement %q: %w", change.Destination, err))
			continue
		}
		if err := os.Rename(change.Backup, change.Destination); err != nil {
			errs = append(errs, fmt.Errorf("restore backup %q: %w", change.Destination, err))
		}
	}
	return errs
}

func reserveSiblingPath(parent string, prefix string) (string, error) {
	file, err := os.CreateTemp(parent, prefix)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func verifyManagedDestination(path string, managed *ManagedState) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat managed destination %q: %w", path, err)
	}
	if managed.Mode == "soft" || (managed.Legacy && managed.Mode == "" && info.Mode()&os.ModeSymlink != 0) {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("managed destination %q was replaced and will not be overwritten", path)
		}
		if managed.Target != "" {
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read managed symlink %q: %w", path, err)
			}
			if target != managed.Target {
				return fmt.Errorf("managed symlink %q was replaced and will not be overwritten", path)
			}
		}
		return nil
	}

	kind := managed.Kind
	checksum := managed.Checksum
	if managed.Legacy && kind == "" {
		if !info.IsDir() {
			return fmt.Errorf("legacy managed destination %q has an unexpected type", path)
		}
		stored, err := os.ReadFile(filepath.Join(path, ".lore-checksum"))
		if err != nil {
			return fmt.Errorf("legacy managed destination %q has no checksum", path)
		}
		kind = "directory"
		checksum = strings.TrimSpace(string(stored))
	}

	if kind == "file" && !info.Mode().IsRegular() {
		return fmt.Errorf("managed destination %q was replaced and will not be overwritten", path)
	}
	if kind == "directory" && !info.IsDir() {
		return fmt.Errorf("managed destination %q was replaced and will not be overwritten", path)
	}
	if kind != "file" && kind != "directory" {
		return fmt.Errorf("managed destination %q has unknown kind %q", path, kind)
	}
	if checksum == "" {
		return fmt.Errorf("managed destination %q has no checksum", path)
	}
	var current string
	if kind == "directory" && managed.ChecksumVersion == 1 {
		current, err = computeLegacyDirChecksum(path)
	} else {
		current, err = ComputePathChecksum(path, kind)
	}
	if err != nil {
		return fmt.Errorf("verify managed destination %q: %w", path, err)
	}
	if current != checksum {
		return fmt.Errorf("destination %q has local modifications", path)
	}
	return nil
}

func ComputeFileChecksum(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%o", info.Mode().Perm())))
	hash.Write([]byte{0})
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func ComputePathChecksum(path string, kind string) (string, error) {
	if kind == "file" {
		return ComputeFileChecksum(path)
	}
	if kind == "directory" {
		return ComputeDirChecksum(path)
	}
	return "", fmt.Errorf("unknown item kind %q", kind)
}

func ComputeDirChecksum(dir string) (string, error) {
	type checksumEntry struct {
		path string
		kind byte
		mode fs.FileMode
	}

	var entries []checksumEntry
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		kind := byte('o')
		switch {
		case info.IsDir():
			kind = 'd'
		case info.Mode().IsRegular():
			kind = 'f'
		case info.Mode()&os.ModeSymlink != 0:
			kind = 'l'
		}
		entries = append(entries, checksumEntry{path: rel, kind: kind, mode: info.Mode()})
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	hash := sha256.New()
	for _, entry := range entries {
		hash.Write([]byte(filepath.ToSlash(entry.path)))
		hash.Write([]byte{0, entry.kind, 0})
		hash.Write([]byte(fmt.Sprintf("%o", entry.mode.Perm())))
		hash.Write([]byte{0})
		path := filepath.Join(dir, entry.path)
		switch entry.kind {
		case 'f':
			file, err := os.Open(path)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(hash, file); err != nil {
				file.Close()
				return "", err
			}
			if err := file.Close(); err != nil {
				return "", err
			}
		case 'l':
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			hash.Write([]byte(target))
		}
		hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func computeLegacyDirChecksum(dir string) (string, error) {
	hash := sha256.New()
	legacyMarker := filepath.Join(dir, ".lore-checksum")
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path == legacyMarker || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		file, err := os.Open(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		if _, err := io.Copy(hash, file); err != nil {
			file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func copyDir(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src string, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
