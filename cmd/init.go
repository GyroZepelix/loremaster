package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/provider"
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

	var selected provider.Provider

	switch len(detected) {
	case 1:
		selected = detected[0]
	case 0:
		// No provider directories found — let user choose which to set up
		all := provider.All()
		fmt.Println("No AI tool directory detected. Select a provider to initialize:")
		for i, p := range all {
			fmt.Printf("  [%d] %s\n", i+1, p.Name())
		}
		fmt.Print("Choice: ")
		var choice int
		if _, err := fmt.Scanf("%d", &choice); err != nil || choice < 1 || choice > len(all) {
			return fmt.Errorf("invalid selection")
		}
		selected = all[choice-1]

		// Create the provider's marker directory
		markerPath := filepath.Join(cwd, selected.MarkerDir())
		if err := os.MkdirAll(markerPath, 0755); err != nil {
			return fmt.Errorf("create %s directory: %w", selected.MarkerDir(), err)
		}
	default:
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
