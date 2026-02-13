package cmd

import (
	"io/fs"
	"os"
	stdpath "path"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/stefan65535/helmer/internal/domain"
	"github.com/stefan65535/helmer/internal/logger"
)

func init() {
	rootCmd.AddCommand(templateCmd)
	templateCmd.Flags().StringVar(&OutputDir, "output-dir", "", "set target root output directory to write rendered templates to. If not set current working directory will be used")
	templateCmd.Flags().BoolVarP(&Verbose, "verbose", "v", false, "enable verbose output")
}

var OutputDir string
var Verbose bool

var templateCmd = &cobra.Command{
	Use:   "template config...",
	Short: "Render Helm chart templates locally",
	Long:  "Render Helm chart templates locally. \nConfiguration is read from one or more Yaml config files. If a directory argument is given Helmer will recursively go looking for Yaml files in the directory structure\nOutput is writen to files in a directory structure according to target path",
	Args:  cobra.MinimumNArgs(1),

	Run: func(cmd *cobra.Command, args []string) {

		if Verbose {
			logger.Default.Level = logger.VERBOSE
		}

		for _, arg := range args {
			err := walkDir(arg, []string{".yml", ".yaml"}, processConfig)
			if err != nil {
				logger.Error(err)
				os.Exit(1)
			}
		}
	},
}

// processConfig loads a config file at path and writes its targets.
func processConfig(path string) error {
	domain.GlobalValues = domain.Values{}

	// TODO add option to set from command line
	domain.GlobalRelease = domain.Release{
		Name:      "release-name",      // This is the default name Helm uses if none is provided.
		Namespace: "release-namespace", // This is the default namespace Helm uses if none is provided.
	}

	// TODO add option to set from command line
	domain.GlobalCapabilities = domain.Capabilities{}

	doc, err := domain.LoadDocument(nil, path)
	if err != nil {
		return err
	}

	err = domain.GlobalValues.ResolveValueRefs()
	if err != nil {
		return err
	}

	err = doc.ResolveChartValueRefs()
	if err != nil {
		return err
	}

	domain.GlobalValues["Helmer"] = map[string]any{
		"target": map[string]any{
			"path": doc.Target.Path,
		},
	}

	err = doc.RenderTarget()
	if err != nil {
		return err
	}

	err = doc.WriteTarget(OutputDir)
	if err != nil {
		return err
	}

	return nil
}

// walkDir recursivly descends path and calls fileFn on every file with a file extension matching one of the extensions in exts.
func walkDir(path string, exts []string, fileFn func(path string) error) error {
	return filepath.WalkDir(path, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() {
			logger.Verbosef(0, "Descending dir %v", path)
		} else {
			ext := stdpath.Ext(path)

			for _, e := range exts {
				if ext == e {
					logger.Verbosef(1, "Reading config %v", path)
					err := fileFn(path)
					if err != nil {
						return err
					}

					return nil
				}
			}
		}

		return nil
	})
}
