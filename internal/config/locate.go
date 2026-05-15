package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/GyroZepelix/loremaster/internal/provider"
)

var profilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ConfigFileName returns the config filename for the given profile.
// Empty string or "default" returns "lore.yml"; otherwise "lore-<profile>.yml".
// The profile must pass validation (see validateProfileName) unless it is empty or "default".
func ConfigFileName(profile string) (string, error) {
	if profile == "" || profile == "default" {
		return "lore.yml", nil
	}
	if err := validateProfileName(profile); err != nil {
		return "", err
	}
	return fmt.Sprintf("lore-%s.yml", profile), nil
}

func validateProfileName(profile string) error {
	if len(profile) > 64 {
		return fmt.Errorf("invalid profile name %q: must match [a-z0-9][a-z0-9_-]* and be at most 64 characters", profile)
	}
	if !profilePattern.MatchString(profile) {
		return fmt.Errorf("invalid profile name %q: must match [a-z0-9][a-z0-9_-]* and be at most 64 characters", profile)
	}
	return nil
}

// LocateProfile finds the config file for the given profile in dir.
// Empty or "default" profile delegates to Locate.
func LocateProfile(dir, profile string) (string, error) {
	if profile == "" || profile == "default" {
		return Locate(dir)
	}
	filename, err := ConfigFileName(profile)
	if err != nil {
		return "", err
	}
	candidates := configCandidates(dir, filename)
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no %s found in %s (run 'lore init' first)", filename, dir)
}

func Locate(dir string) (string, error) {
	candidates := configCandidates(dir, "lore.yml")

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no lore.yml found in %s (run 'lore init' first)", dir)
}

func configCandidates(dir, filename string) []string {
	candidates := []string{filepath.Join(dir, filename)}
	for _, configDir := range provider.ConfigDirs() {
		candidates = append(candidates, filepath.Join(dir, configDir, filename))
	}
	return candidates
}
