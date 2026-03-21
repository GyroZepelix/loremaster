package gitignore

import (
	"os"
	"strings"
)

const header = "# Managed by loremaster"

func EnsureEntries(gitignorePath string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	existing := make(map[string]bool)
	for _, line := range lines {
		existing[strings.TrimSpace(line)] = true
	}

	var toAdd []string
	for _, entry := range entries {
		if !existing[entry] {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	hasHeader := existing[header]

	if hasHeader {
		// Insert new entries right after the managed section
		var result []string
		inSection := false
		inserted := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == header {
				inSection = true
				result = append(result, line)
				continue
			}
			if inSection && !inserted {
				// Still in managed section — keep going until we hit a blank or non-managed line
				if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
					result = append(result, line)
					continue
				}
				// End of managed section — insert new entries here
				for _, entry := range toAdd {
					result = append(result, entry)
				}
				inserted = true
			}
			result = append(result, line)
		}
		// If we were in section but never hit the end (section is at EOF)
		if inSection && !inserted {
			for _, entry := range toAdd {
				result = append(result, entry)
			}
		}

		output := strings.Join(result, "\n")
		output = strings.TrimRight(output, "\n")
		if output != "" {
			output += "\n"
		}
		return os.WriteFile(gitignorePath, []byte(output), 0644)
	}

	// No header yet — append section at the end
	var result strings.Builder
	existingContent := strings.TrimRight(string(content), "\n")
	if existingContent != "" {
		result.WriteString(existingContent)
		result.WriteString("\n\n")
	}

	result.WriteString(header)
	result.WriteString("\n")
	for _, entry := range toAdd {
		result.WriteString(entry)
		result.WriteString("\n")
	}

	return os.WriteFile(gitignorePath, []byte(result.String()), 0644)
}

func RemoveEntries(gitignorePath string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	toRemove := make(map[string]bool)
	for _, e := range entries {
		toRemove[e] = true
	}

	lines := strings.Split(string(content), "\n")
	var result []string
	for _, line := range lines {
		if !toRemove[strings.TrimSpace(line)] {
			result = append(result, line)
		}
	}

	// Check if any managed entries remain after the header
	hasManagedEntries := false
	foundHeader := false
	for _, line := range result {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			foundHeader = true
			continue
		}
		if foundHeader && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			hasManagedEntries = true
			break
		}
	}

	// Remove header if no managed entries remain
	if !hasManagedEntries {
		var cleaned []string
		for _, line := range result {
			if strings.TrimSpace(line) != header {
				cleaned = append(cleaned, line)
			}
		}
		result = cleaned
	}

	output := strings.Join(result, "\n")
	output = strings.TrimRight(output, "\n")
	if output != "" {
		output += "\n"
	}

	return os.WriteFile(gitignorePath, []byte(output), 0644)
}

func ManagedEntries(gitignorePath string) ([]string, error) {
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	inSection := false
	var entries []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			inSection = true
			continue
		}
		if inSection {
			if trimmed == "" || (strings.HasPrefix(trimmed, "#") && trimmed != header) {
				break
			}
			entries = append(entries, trimmed)
		}
	}

	return entries, nil
}
