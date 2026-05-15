package provider

import (
	"os"
	"path/filepath"
)

func Detect(projectRoot string) ([]Provider, error) {
	var detected []Provider
	seen := make(map[string]bool)
	for _, p := range All() {
		for _, marker := range p.MarkerDirs() {
			markerPath := filepath.Join(projectRoot, marker)
			if info, err := os.Stat(markerPath); err == nil && info.IsDir() {
				if !seen[p.Name()] {
					detected = append(detected, p)
					seen[p.Name()] = true
				}
				break
			}
		}
	}
	return detected, nil
}
