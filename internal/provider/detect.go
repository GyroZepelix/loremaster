package provider

import (
	"os"
	"path/filepath"
)

func Detect(projectRoot string) ([]Provider, error) {
	var detected []Provider
	for _, p := range All() {
		markerPath := filepath.Join(projectRoot, p.MarkerDir())
		if info, err := os.Stat(markerPath); err == nil && info.IsDir() {
			detected = append(detected, p)
		}
	}
	return detected, nil
}
