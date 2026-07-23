package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// IncludeEntry represents a parsed include directive with source and destination paths.
// Src is the path within the skill repository; Dst is the path where it will be placed.
type IncludeEntry struct {
	Src string
	Dst string
}

// ParseIncludeEntry parses a raw include string into an IncludeEntry.
// Format: "src" (identity mapping) or "src:dst" (remap). Splits on first colon only.
func ParseIncludeEntry(raw string) (IncludeEntry, error) {
	var src, dst string

	if idx := strings.IndexByte(raw, ':'); idx >= 0 {
		src = raw[:idx]
		dst = raw[idx+1:]
	} else {
		src = raw
		dst = raw
	}

	if err := validateIncludePath(src, "source"); err != nil {
		return IncludeEntry{}, err
	}
	if err := validateIncludePath(dst, "destination"); err != nil {
		return IncludeEntry{}, err
	}

	return IncludeEntry{Src: filepath.Clean(src), Dst: filepath.Clean(dst)}, nil
}

func ValidateResourceName(raw string) (string, error) {
	if err := validateIncludePath(raw, "resource"); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Clean(raw)), nil
}

func joinResourcePath(resource, destination string) string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(resource), destination))
}

// validateIncludePath validates a single path component (source or destination) of an include entry.
func validateIncludePath(path, side string) error {
	if path == "" || path == "." {
		return fmt.Errorf("invalid include %s: must not be empty", side)
	}

	// Reject control characters including null bytes (filesystem safety)
	for _, c := range path {
		if c < 0x20 {
			return fmt.Errorf("invalid include path %q: control characters are not allowed", path)
		}
	}

	// Reject backslashes before any other check (cross-platform safety)
	if strings.Contains(path, `\`) {
		return fmt.Errorf("invalid include path %q: backslashes are not allowed", path)
	}
	if strings.ContainsAny(path, "*?[]") {
		return fmt.Errorf("invalid include path %q: glob metacharacters are not allowed", path)
	}

	cleaned := filepath.Clean(path)

	if filepath.IsAbs(cleaned) {
		return fmt.Errorf("invalid include path %q: must be relative", path)
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid include %s path %q: must not escape root", side, path)
	}

	if strings.Contains(cleaned, ":") {
		return fmt.Errorf("invalid include path %q: colons are not allowed", path)
	}

	return nil
}

// ValidateOverlaps checks a slice of IncludeEntry values for destination conflicts.
// It returns an error if any two entries share the same Dst or if one Dst is a prefix of another.
func ValidateOverlaps(entries []IncludeEntry) error {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			a := entries[i].Dst
			b := entries[j].Dst

			if a == b {
				return fmt.Errorf("duplicate include destination %q", a)
			}
			if strings.HasPrefix(a+"/", b+"/") {
				return fmt.Errorf("overlapping include destinations: %q is a prefix of %q", b, a)
			}
			if strings.HasPrefix(b+"/", a+"/") {
				return fmt.Errorf("overlapping include destinations: %q is a prefix of %q", a, b)
			}
		}
	}
	return nil
}
