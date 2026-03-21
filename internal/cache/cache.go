package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func Dir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "loremaster"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty (is $HOME set?)")
	}
	return filepath.Join(home, ".local", "share", "loremaster"), nil
}

var sshPattern = regexp.MustCompile(`^git@([^:]+):(.+)$`)

func NormalizeURL(rawURL string) string {
	s := rawURL

	// Handle git@host:path format
	if m := sshPattern.FindStringSubmatch(s); m != nil {
		s = m[1] + "/" + m[2]
	} else {
		// Strip protocol
		for _, prefix := range []string{"https://", "http://", "git://", "ssh://"} {
			if strings.HasPrefix(s, prefix) {
				s = strings.TrimPrefix(s, prefix)
				break
			}
		}
		// Strip user@ prefix (e.g. git@host/path from ssh:// URLs)
		if idx := strings.Index(s, "@"); idx != -1 {
			slashIdx := strings.Index(s, "/")
			if slashIdx == -1 || idx < slashIdx {
				s = s[idx+1:]
			}
		}
	}

	// Strip trailing .git
	s = strings.TrimSuffix(s, ".git")

	// Lowercase the host portion
	if idx := strings.Index(s, "/"); idx != -1 {
		s = strings.ToLower(s[:idx]) + s[idx:]
	} else {
		s = strings.ToLower(s)
	}

	return s
}

func RepoDir(rawURL string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	normalized := NormalizeURL(rawURL)
	hash := sha256.Sum256([]byte(normalized))
	return filepath.Join(dir, fmt.Sprintf("%x", hash[:16])), nil
}

func SkillPath(rawURL string, skillName string) (string, error) {
	repoDir, err := RepoDir(rawURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(repoDir, skillName), nil
}

func EnsureDir() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}
