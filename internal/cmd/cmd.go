package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "helmer",
	Short: "Helmer is a utility that can process multiple Helm charts in multiple configuraions",
	Long:  `Helmer is a utility that can process multiple Helm charts in multiple configuraions`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
		os.Exit(1)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `All software has versions. This is mine`,
	Run: func(cmd *cobra.Command, args []string) {
		buildInfo, _ := debug.ReadBuildInfo()

		fmt.Println(buildInfo.Main.Version)
	},
}
