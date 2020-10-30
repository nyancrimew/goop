package cmd

import (
	"fmt"
	"github.com/deletescape/goop/pkg/goop"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use: "goop",
	Short: "goop is a very fast tool to grab sources from exposed .git folders",
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var dir string
		if len(args) >= 2 {
			dir = args[1]
		}
		if err := goop.Clone(args[0], dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
