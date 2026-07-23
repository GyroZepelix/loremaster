package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "0.4.1"

var rootCmd = &cobra.Command{
	Use:   "lore",
	Short: "Loremaster - declarative AI resource syncer",
	Long:  "Loremaster syncs skills, prompts, commands, and other resources into provider configuration directories.",
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("lore version {{.Version}}\n")
	rootCmd.SilenceUsage = true

	completionCmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}

	completionCmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		},
	})

	completionCmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenZshCompletion(os.Stdout)
		},
	})

	completionCmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenFishCompletion(os.Stdout, true)
		},
	})

	rootCmd.AddCommand(completionCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
