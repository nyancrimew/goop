package cmd

import (
	"os"

	"github.com/deletescape/goop/pkg/goop"
	"github.com/phuslu/log"
	"github.com/spf13/cobra"
)

var force bool
var keep bool
var list bool
var useragent string
var rootCmd = &cobra.Command{
	Use:   "goop",
	Short: "goop is a very fast tool to grab sources from exposed .git folders",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var dir string
		if len(args) >= 2 {
			dir = args[1]
		}
		if list {
			if err := goop.CloneList(args[0], dir, force, keep, useragent); err != nil {
				log.Error().Err(err).Msg("exiting")
				os.Exit(1)
			}
		} else {
			if err := goop.Clone(args[0], dir, force, keep, useragent); err != nil {
				log.Error().Err(err).Msg("exiting")
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&force, "force", "f", false, "overrides DIR if it already exists")
	rootCmd.PersistentFlags().BoolVarP(&keep, "keep", "k", false, "keeps already downloaded files in DIR, useful if you keep being ratelimited by server")
	rootCmd.PersistentFlags().BoolVarP(&list, "list", "l", false, "allows you to supply the name of a file containing a list of domain names instead of just one domain")
	rootCmd.PersistentFlags().StringVarP(&useragent, "useragent", "u", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.102 Safari/537.36", "allows you to change the default User-Agent")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("exiting")
		os.Exit(1)
	}
}
