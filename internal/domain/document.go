package domain

import (
	"fmt"
	"os"
	stdpath "path"

	"github.com/goccy/go-yaml"
	"github.com/stefan65535/helmer/internal/logger"
	"github.com/stefan65535/helmer/internal/utils"
)

type Document struct {
	Includes     []*Include   `yaml:"includes"`
	Charts       []*Chart     `yaml:"charts"`
	Values       Values       `yaml:"values"`
	Capabilities Capabilities `yaml:"capabilities,omitempty"`
	Release      Release      `yaml:"release,omitempty"` // TODO remove?
	Target       *Target      `yaml:"target,omitempty"`

	parent *Document
	path   string // Path to the config file, used for detecting circular includes

}

type Include struct {
	Path           string    `yaml:"path"`
	loadedDocument *Document // Loaded document after resolving the include
}

func LoadDocument(parent *Document, path string, indent int) (*Document, error) {
	logger.Verbosef(indent, "Loading document %v", path)

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

	GlobalValues = utils.MergeMaps(doc.Values, GlobalValues)

	if err = doc.ResolveDependencies(path, indent+1); err != nil {
		return nil, err
	}

	setGlobalCapsAndRelease(&doc)

	return &doc, nil
}

// ResolveDependencies loads includes and charts
func (d *Document) ResolveDependencies(path string, indent int) error {
	if err := d.loadCharts(stdpath.Dir(path), indent); err != nil {
		return err
	}

	if err := d.resolveIncludes(path, indent); err != nil {
		return err
	}

	if err := d.Values.ResolveValueFileAndExternalRefs(stdpath.Dir(path)); err != nil {
		return err
	}

	return nil
}

func (d *Document) resolveIncludes(basePath string, indent int) error {
	if len(d.Includes) == 0 {
		return nil
	}

	logger.Verbose(indent, "Loading includes")
	for _, include := range d.Includes {
		path := stdpath.Join(stdpath.Dir(basePath), include.Path)

		includedDocument, err := LoadDocument(d, path, indent+1)
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

func (d *Document) loadCharts(path string, indent int) error {
	if len(d.Charts) == 0 {
		return nil
	}

	logger.Verbose(indent, "Loading charts")
	for _, chart := range d.Charts {
		logger.Verbosef(indent+1, "Loading chart %v", chart.Path)
		if err := chart.load(path); err != nil {
			return err
		}

		if err := chart.Values.ResolveValueFileAndExternalRefs(path); err != nil {
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

	helmerValues := HelmerValues{
		Target: HelmerTarget{
			Path: d.Target.Path,
		},
	}
	for _, chart := range docCharts {
		for _, sd := range helmerValues.Target.SubDirs {
			if sd == chart.TargetDir {
				goto SkipAppend
			}
		}
		helmerValues.Target.SubDirs = append(helmerValues.Target.SubDirs, chart.TargetDir)
	SkipAppend:
	}

	hvDoc, err := yaml.Marshal(helmerValues)
	if err != nil {
		return fmt.Errorf("error marshaling helmer values: %w", err)
	}

	var hv any
	err = yaml.Unmarshal(hvDoc, &hv)
	if err != nil {
		return fmt.Errorf("error unmarshaling helmer values: %w", err)
	}
	GlobalValues["Helmer"] = hv


	logger.Verbosef(1, "Rendering target %v", d.Target.Path)
	logger.Verbosef(2, "Global values: %+v", GlobalValues)

	for _, chart := range docCharts {
		logger.Verbosef(2, "Rendering chart %v", chart.Path)

		release, err := chart.render()
		if err != nil {
			return err
		}
		d.Target.renderedReleases = append(d.Target.renderedReleases, RenderedRelease{Release: release, TargetDir: chart.TargetDir})
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
