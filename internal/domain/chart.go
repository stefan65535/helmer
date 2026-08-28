package domain

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	stdpath "path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/stefan65535/helmer/internal/utils"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/loader"

	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

type Chart struct {
	Path         string      `yaml:"path"`
	Patches      []*Patch    `yaml:"patches"`
	Values       Values      `yaml:"values"`
	Release      Release     `yaml:"release,omitempty"`
	TargetDir    string      `yaml:"targetDir,omitempty"`
	AuxTemplates []*Template `yaml:"auxTemplates,omitempty"`

	charter chart.Charter
}

type Template struct {
	Path   string `yaml:"path"`
	Values Values `yaml:"values"`

	loadedTemplate   []byte
	renderedTemplate []byte
}

type RenderedChart struct {
	Name      string
	Manifests []string
}

// TODO chash charts instead of charters? Charts are what we load from disk and render, charters are what Helm uses to render. Charts could also include the loaded charter and rendered release to avoid having to pass them around
var charterCash = make(map[string]chart.Charter)

func (c *Chart) load(configPath string) error {
	if c.charter == nil {
		absPath, err := filepath.Abs(stdpath.Join(configPath, c.Path))
		if err != nil {
			return err
		}

		var charter chart.Charter

		// Try to get the charter from cache
		if cachedCharter, ok := charterCash[absPath]; ok {
			charter = cachedCharter
		} else {
			loader, err := loader.Loader(absPath)
			if err != nil {
				return err
			}

			charter, err = loader.Load()
			if err != nil {
				return err
			}

			charterCash[absPath] = charter
		}
		c.charter = charter
	}

	if c.TargetDir == "" {
		accessor, err := chart.NewAccessor(c.charter)
		if err != nil {
			return err
		}

		c.TargetDir = accessor.Name()
	}

	for _, auxTemplate := range c.AuxTemplates {
		auxAbsPath, err := filepath.Abs(stdpath.Join(configPath, auxTemplate.Path))
		if err != nil {
			return err
		}

		auxTemplateContent, err := os.ReadFile(auxAbsPath)
		if err != nil {
			return err
		}

		auxTemplate.loadedTemplate = auxTemplateContent
	}

	return nil
}

func (c *Chart) render(d *Document) (*releasev1.Release, error) {
	if c.charter == nil {
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
	if c.Release.Name != "" {
		install.ReleaseName = c.Release.Name
	} else {
		install.ReleaseName = GlobalRelease.Name
	}
	if c.Release.Namespace != "" {
		install.Namespace = c.Release.Namespace
	} else {
		install.Namespace = GlobalRelease.Namespace
	}

	values := utils.MergeMaps(GlobalValues, c.Values)

	releaser, err := install.Run(c.charter, values)
	if err != nil {
		return nil, err
	}

	release := releaser.(*releasev1.Release) // Helm does not provide any public help to deal with Releaser. releaserToV1Release exists in get_values.go but it's a private function.

	chartInfo := &strings.Builder{}
	chartInfo.WriteString(fmt.Sprintf("# Config: %v\n", d.Path))
	chartInfo.WriteString(fmt.Sprintf("# Chart: %v\n", c.Path))

	if err = applyPatches(release, c.Patches, values); err != nil {
		return nil, err
	}

	release.Manifest = chartInfo.String() + release.Manifest

	err = c.renderAuxTemplates(values)
	if err != nil {
		return nil, err
	}

	for _, auxTemplate := range c.AuxTemplates {
		release.Manifest = release.Manifest + "---\n"
		release.Manifest = release.Manifest + "# Source: " + auxTemplate.Path + "\n"
		release.Manifest = release.Manifest + string(auxTemplate.renderedTemplate)
	}

	return release, nil
}

func (c *Chart) renderAuxTemplates(values map[string]any) error {
	for _, auxTemplate := range c.AuxTemplates {
		localValues := utils.MergeMaps(values, auxTemplate.Values)

		tmpl, err := template.New("tpl").Parse(string(auxTemplate.loadedTemplate))
		if err != nil {
			return err
		}

		values := map[string]any{
			"Release": c.Release,
			"Values":  localValues,
		}

		var rendered bytes.Buffer
		if err := tmpl.Execute(&rendered, values); err != nil {
			return err
		}

		auxTemplate.renderedTemplate = rendered.Bytes()
	}

	return nil
}

func applyPatches(release *releasev1.Release, patches []*Patch, values map[string]any) error {
	if len(patches) == 0 {
		return nil
	}

	patchInfo := &strings.Builder{}
	patchInfo.WriteString("# Patches applied:\n")

	for _, patch := range patches {
		newManifest, err := patch.Apply(release.Manifest, values)
		if err != nil {
			return err
		}
		patchInfo.WriteString("#   - Target:\n")
		if patch.Target.Kind != "" {
			patchInfo.WriteString(fmt.Sprintf("#       Kind: %v\n", patch.Target.Kind))
		}
		if patch.Target.Name != "" {
			patchInfo.WriteString(fmt.Sprintf("#       Name: %v\n", patch.Target.Name))
		}
		if patch.Target.Namespace != "" {
			patchInfo.WriteString(fmt.Sprintf("#       Namespace: %v\n", patch.Target.Namespace))
		}
		patchInfo.WriteString("#     Path:\n")
		for _, patch := range patch.PatchJSON6902 {
			patchInfo.WriteString(fmt.Sprintf("#       - %v\n", patch.Path.String()))
		}

		release.Manifest = newManifest
	}

	release.Manifest = patchInfo.String() + release.Manifest

	return nil
}
