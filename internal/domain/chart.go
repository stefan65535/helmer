package domain

import (
	"errors"
	stdpath "path"
	"path/filepath"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

type Chart struct {
	Path    string   `yaml:"path"`
	Patches []*Patch `yaml:"patches"`
	Values  Values   `yaml:"values"`

	ConfigPath string `yaml:"-"` // TODO What is this?

	loadedChart *chartv2.Chart
}

type RenderedChart struct {
	Name      string
	Manifests []string
}

var chartCash = make(map[string]*chartv2.Chart)

func (c *Chart) load(configPath string) error {
	c.ConfigPath = configPath

	if c.loadedChart == nil {
		absPath, err := filepath.Abs(stdpath.Join(configPath, c.Path))
		if err != nil {
			return err
		}

		var chart *chartv2.Chart

		// Try to get the chart from cache
		if cachedChart, ok := chartCash[absPath]; ok {
			chart = cachedChart
		} else {
			chart, err = loader.Load(absPath)
			if err != nil {
				return err
			}
			chartCash[absPath] = chart
		}
		c.loadedChart = chart
	}

	return nil
}

func (c *Chart) render(values map[string]any) (*releasev1.Release, error) {
	if c.loadedChart == nil {
		return nil, errors.New("chart not loaded")
	}

	cfg := action.Configuration{}
	cfg.Capabilities = common.DefaultCapabilities

	cfg.Capabilities.APIVersions = GlobalCapabilities.APIVersions
	cfg.Capabilities.KubeVersion.Version = GlobalCapabilities.KubeVersion.Version
	cfg.Capabilities.KubeVersion.Major = GlobalCapabilities.KubeVersion.Major
	cfg.Capabilities.KubeVersion.Minor = GlobalCapabilities.KubeVersion.Minor

	install := action.NewInstall(&cfg)
	install.DryRunStrategy = action.DryRunClient
	install.ReleaseName = GlobalRelease.Name
	install.Namespace = GlobalRelease.Namespace

	localValues := loader.MergeMaps(values, c.Values)

	releaser, err := install.Run(c.loadedChart, localValues)
	if err != nil {
		return nil, err
	}

	release := releaser.(*releasev1.Release) // Helm does not provide any public help to deal with Releaser. releaserToV1Release exists in get_values.go but it's a private function.

	if err = applyPatches(release, c.Patches, localValues); err != nil {
		return nil, err
	}

	return release, nil
}

func applyPatches(release *releasev1.Release, patches []*Patch, values map[string]any) error {
	if len(patches) == 0 {
		return nil
	}

	for _, patch := range patches {
		newManifest, err := patch.Apply(release.Manifest, values)
		if err != nil {
			return err
		}
		release.Manifest = newManifest
	}

	return nil
}
