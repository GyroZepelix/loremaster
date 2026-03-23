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

func LinkSkill(src string, dst string, linkType string) error {
	if linkType == "soft" {
		// Remove existing destination (symlink or directory)
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("remove existing %q: %w", dst, err)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("create parent dir for %q: %w", dst, err)
		}

		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("symlink %q -> %q: %w", dst, src, err)
		}
		return nil
	}

	// Hard copy
	checksumFile := filepath.Join(dst, ".lore-checksum")

	if _, err := os.Stat(dst); err == nil {
		// Destination exists — check if managed by loremaster
		storedChecksum, readErr := os.ReadFile(checksumFile)
		if readErr != nil {
			// No .lore-checksum — not managed by loremaster, skip to prevent data loss
			fmt.Fprintf(os.Stderr, "warning: skipping %q: directory exists but is not managed by loremaster (no .lore-checksum found). Remove it manually to allow sync.\n", dst)
			return nil
		}
		// Managed — check for local modifications
		currentChecksum, err := ComputeDirChecksum(dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: could not verify checksum: %s\n", dst, err)
			return nil
		}
		if strings.TrimSpace(string(storedChecksum)) != currentChecksum {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: local modifications detected\n", dst)
			return nil
		}
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("remove existing %q: %w", dst, err)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create parent dir for %q: %w", dst, err)
	}

	// Copy directory recursively
	if err := copyDir(src, dst); err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}

	// Compute and store checksum
	checksum, err := ComputeDirChecksum(dst)
	if err != nil {
		return fmt.Errorf("compute checksum for %q: %w", dst, err)
	}

	return os.WriteFile(checksumFile, []byte(checksum), 0644)
}

func ComputeDirChecksum(dir string) (string, error) {
	h := sha256.New()

	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip the checksum file itself
		if filepath.Base(path) == ".lore-checksum" {
			return nil
		}
		// Skip symlinks
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(paths)

	for _, rel := range paths {
		f, err := os.Open(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		// Write filename with null separator to prevent collisions
		h.Write([]byte(rel))
		h.Write([]byte{0})
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func copyDir(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		// Skip symlinks in source to prevent following links to arbitrary locations
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
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
