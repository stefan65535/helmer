package domain

import (
	"fmt"
	"os"
	stdpath "path"

	"github.com/goccy/go-yaml"
	"github.com/stefan65535/helmer/internal/logger"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

type Document struct {
	Includes     []*Include   `yaml:"includes"`
	Charts       []*Chart     `yaml:"charts"`
	Values       Values       `yaml:"values"`
	Capabilities Capabilities `yaml:"capabilities,omitempty"`
	Release      Release      `yaml:"release,omitempty"`
	Target       *Target      `yaml:"target,omitempty"`

	parent *Document
	path   string // Path to the config file, used for detecting circular includes

}

type Include struct {
	Path           string    `yaml:"path"`
	loadedDocument *Document // Loaded document after resolving the include
}

func LoadDocument(parent *Document, path string) (*Document, error) {
	// Check for circular includes
	for p := parent; p != nil; p = p.parent {
		if p.path == path {
			return nil, fmt.Errorf("circular include detected: %v", path)
		}
	}

	// Load
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var doc Document
	decoder := yaml.NewDecoder(file, yaml.DisallowUnknownField())
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("error decoding %v:\n%w", path, err)
	}
	doc.parent = parent
	doc.path = path

	GlobalValues = loader.MergeMaps(doc.Values, GlobalValues)

	if err = doc.ResolveDependencies(path); err != nil {
		return nil, err
	}

	setGlobalCapsAndRelease(&doc)

	return &doc, nil
}

// ResolveDependencies loads includes and charts
func (d *Document) ResolveDependencies(path string) error {
	if err := d.resolveIncludes(path); err != nil {
		return err
	}

	if err := d.loadCharts(stdpath.Dir(path)); err != nil {
		return err
	}

	return nil
}

func (d *Document) resolveIncludes(basePath string) error {
	for _, include := range d.Includes {
		path := stdpath.Join(stdpath.Dir(basePath), include.Path)

		includedDocument, err := LoadDocument(d, path)
		if err != nil {
			return fmt.Errorf("error resolving includes in %v:\n%w", basePath, err)
		}

		if includedDocument.Target != nil {
			return fmt.Errorf("included config %v contains a target, which is not supported", path)
		}

		include.loadedDocument = includedDocument
	}

	return nil
}

func (d *Document) loadCharts(configPath string) error {
	for _, chart := range d.Charts {
		if err := chart.load(configPath); err != nil {
			return err
		}
	}

	return nil
}

// CollectCharts recursively collects all Charts from this document and its included documents.
func (d *Document) CollectCharts() []*Chart {
	var charts []*Chart
	charts = append(charts, d.Charts...)

	for _, include := range d.Includes {
		charts = append(charts, include.loadedDocument.CollectCharts()...)
	}

	return charts
}

func (d *Document) ResolveChartValueFileAndExternalRefs(path string) error {
	for _, chart := range d.Charts {
		if err := chart.Values.ResolveValueFileAndExternalRefs(path); err != nil {
			return err
		}
	}

	for _, include := range d.Includes {
		if err := include.loadedDocument.ResolveChartValueFileAndExternalRefs(path); err != nil {

			return err
		}
	}

	return nil
}

func (d *Document) ResolveChartValueRefs() error {
	for _, chart := range d.Charts {
		if err := chart.Values.ResolveValueRefs(); err != nil {
			return err
		}
	}

	for _, include := range d.Includes {
		if err := include.loadedDocument.ResolveChartValueRefs(); err != nil {

			return err
		}
	}

	return nil
}

func (d *Document) RenderTarget() error {
	docCharts := d.CollectCharts()

	if d.Target != nil {
		logger.Verbosef(2, "Processing target %v in %v", d.Target.Path, d.path)

		for _, chart := range docCharts {
			logger.Verbosef(3, "Rendering chart %v", chart.Path)

			release, err := chart.render(GlobalValues)
			if err != nil {
				return err
			}
			d.Target.renderedReleases = append(d.Target.renderedReleases, release)
		}
	}

	return nil
}

func (d *Document) WriteTarget(outputDir string) error {
	if d.Target != nil {
		if err := d.Target.write(outputDir); err != nil {
			return err
		}
	}

	return nil
}
