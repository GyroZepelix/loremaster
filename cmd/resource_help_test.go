package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSkeletonDocumentsDynamicResources(t *testing.T) {
	project := t.TempDir()
	os.MkdirAll(filepath.Join(project, ".claude"), 0755)
	withWorkingDirectory(t, project)
	oldProfile := initProfileFlag
	t.Cleanup(func() { initProfileFlag = oldProfile })
	initProfileFlag = ""

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(project, "lore.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"skills:", "# prompts:", "# hooks/tools:", "literal provider-relative resource path"} {
		if !strings.Contains(string(content), expected) {
			t.Errorf("skeleton missing %q:\n%s", expected, content)
		}
	}
}

func TestCommandHelpUsesResourceTerminology(t *testing.T) {
	if !strings.Contains(rootCmd.Short, "resource") || !strings.Contains(syncCmd.Short, "resources") {
		t.Fatalf("root short = %q, sync short = %q", rootCmd.Short, syncCmd.Short)
	}
	prune := syncCmd.Flags().Lookup("prune")
	if prune == nil || !strings.Contains(prune.Usage, "resources") {
		t.Fatalf("prune usage = %#v", prune)
	}
}
