package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgjalic/loremaster/internal/config"
	"github.com/dgjalic/loremaster/internal/provider"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a lore.yml configuration file",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Check if lore.yml already exists
	if existing, err := config.Locate(cwd); err == nil {
		fmt.Printf("lore.yml already exists at %s\n", existing)
		return nil
	}

	// Detect providers
	detected, err := provider.Detect(cwd)
	if err != nil {
		return err
	}

	if len(detected) == 0 {
		return fmt.Errorf("no supported AI tool detected (looked for .claude/, .opencode/)")
	}

	var selected provider.Provider

	if len(detected) == 1 {
		selected = detected[0]
	} else {
		// Multiple providers — prompt user
		fmt.Println("Multiple AI tools detected. Select a provider:")
		for i, p := range detected {
			fmt.Printf("  [%d] %s\n", i+1, p.Name())
		}
		fmt.Print("Choice: ")
		var choice int
		if _, err := fmt.Scanf("%d", &choice); err != nil || choice < 1 || choice > len(detected) {
			return fmt.Errorf("invalid selection")
		}
		selected = detected[choice-1]
	}

	// Generate skeleton lore.yml
	skeleton := fmt.Sprintf(`provider: %s
skills:
  # - source: git@github.com:user/skills-repo.git
  #   ref: main
  #   include: [skill-name]
  #   type: soft
`, selected.Name())

	configPath := filepath.Join(cwd, "lore.yml")
	if err := os.WriteFile(configPath, []byte(skeleton), 0644); err != nil {
		return fmt.Errorf("write lore.yml: %w", err)
	}

	fmt.Printf("Created lore.yml for %s\n", selected.Name())
	return nil
}
