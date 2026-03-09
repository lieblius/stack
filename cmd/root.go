package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "st",
	Short: "Stacked PR management tool",
	Long:  "A CLI for managing stacked pull requests with squash merge support.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(trackCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(syncCmd)
}
