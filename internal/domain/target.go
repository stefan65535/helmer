package domain

import (
	"path/filepath"

	"os"
	stdpath "path"

	"github.com/stefan65535/helmer/internal/logger"
	releasev1v1 "helm.sh/helm/v4/pkg/release/v1"
)

type Target struct {
	Path string `yaml:"path"`

	renderedReleases []*releasev1v1.Release
}

// fileCreated is a map that tracks whether a file with a given name has been created.
var fileCreated = make(map[string]bool)

// write writes all rendered releases associated with the Target to the specified base directory.
func (t *Target) write(baseDir string) error {
	dir := stdpath.Join(baseDir, t.Path)

	for _, release := range t.renderedReleases {
		err := writeRelease(dir, release)

		if err != nil {
			return err
		}
	}

	return nil
}

// writeRelease writes the manifest of the given Helm release to a YAML file in the specified directory.
// If the file already exists (tracked by the fileCreated map), it appends to the file; otherwise, it creates a new file.
// The function ensures the target directory exists, handles file creation and opening, and writes the release manifest content.
func writeRelease(dir string, release *releasev1v1.Release) error {
	fileName := stdpath.Join(dir, release.Chart.Metadata.Name, "manifest.yaml")
	logger.Verbosef(3, "Writing %v", fileName)
	absFileName, err := filepath.Abs(fileName)
	if err != nil {
		return err
	}

	var file *os.File
	if _, ok := fileCreated[absFileName]; ok {
		file, err = os.OpenFile(fileName, os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			return err
		}
	} else {
		err = os.MkdirAll(stdpath.Dir(fileName), 0755)
		if err != nil {
			return err
		}

		file, err = os.OpenFile(fileName, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}

		fileCreated[absFileName] = true
	}
	defer file.Close()

	_, err = file.Write([]byte(release.Manifest))
	if err != nil {
		return err
	}

	return nil
}
