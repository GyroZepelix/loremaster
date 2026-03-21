package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func Locate(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "lore.yml"),
		filepath.Join(dir, ".claude", "lore.yml"),
		filepath.Join(dir, ".opencode", "lore.yml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no lore.yml found in %s (run 'lore init' first)", dir)
}
