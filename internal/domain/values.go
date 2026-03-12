package domain

import (
	"errors"
	"fmt"
	"os"
	stdpath "path"
	"path/filepath"

	"github.com/go-openapi/jsonreference"
	"github.com/goccy/go-yaml"
)

var GlobalValues Values
var GlobalCapabilities Capabilities
var GlobalRelease Release

type Values map[string]any

type Capabilities struct {
	APIVersions []string    `yaml:"apiVersions"`
	KubeVersion KubeVersion `yaml:"kubeVersion"`
}

type KubeVersion struct {
	Version string `yaml:"version"`
	Major   string `yaml:"major"`
	Minor   string `yaml:"minor"`
}

type Release struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

func setGlobalCapsAndRelease(doc *Document) {
	if len(doc.Capabilities.APIVersions) > 0 {
		GlobalCapabilities.APIVersions = doc.Capabilities.APIVersions
	}

	if doc.Capabilities.KubeVersion.Version != "" || doc.Capabilities.KubeVersion.Major != "" || GlobalCapabilities.KubeVersion.Minor != "" {
		GlobalCapabilities.KubeVersion.Version = doc.Capabilities.KubeVersion.Version
		GlobalCapabilities.KubeVersion.Major = doc.Capabilities.KubeVersion.Major
		GlobalCapabilities.KubeVersion.Minor = doc.Capabilities.KubeVersion.Minor
	}

	if doc.Release.Name != "" || doc.Release.Namespace != "" {
		GlobalRelease.Name = doc.Release.Name
		GlobalRelease.Namespace = doc.Release.Namespace
	}
}

func (h Values) ResolveValueRefs() error {
	if err := resolveValueRefs(h); err != nil {
		return err
	}

	return nil
}

// resolveValueRefs resolves references pointing to the values structure
func resolveValueRefs(nodes map[string]any) error {
	for i := range nodes {
		if err := resolveValueRefsYamlNode(i, nodes[i]); err != nil {
			return err
		}

		if mapNode, ok := nodes[i].(map[string]any); ok {
			if childNode, ok := mapNode["$ref"]; ok {
				if ref, ok := childNode.(string); ok {
					r, err := jsonreference.New(ref)
					if err != nil {
						return err
					}

					if r.HasFragmentOnly {
						val, _, err := r.GetPointer().Get(GlobalValues)
						if err != nil {
							return fmt.Errorf(`error evaluating reference "%v": %v`, ref, err)
						}
						nodes[i] = val
					} else {
						return errors.New(`$ref field only supports URI fragments pointing to local values`)
					}
				} else {
					return errors.New(`$ref field must have string type`)
				}
			}
		}
	}

	return nil
}

func resolveValueRefsYamlNode(parent string, node any) error {
	if mapNode, ok := node.(map[string]any); ok {
		return resolveValueRefs(mapNode)
	}
	if sequenceNode, ok := node.([]any); ok {
		return resolveValueRefsYamlSequence(parent, sequenceNode)
	}

	return nil
}

func resolveValueRefsYamlSequence(parent string, node []any) error {
	for _, v := range node {
		if err := resolveValueRefsYamlNode(parent, v); err != nil {
			return err
		}
	}

	return nil
}

func (h Values) ResolveValueFileAndExternalRefs(basePath string) error {
	if err := resolveValueFileAndExternalRefs(h, basePath); err != nil {
		return err
	}

	return nil
}

// resolveValueFileAndExternalRefs resolves $file directives and $ref pointing to external files
func resolveValueFileAndExternalRefs(nodes map[string]any, basePath string) error {
	for k, node := range nodes {
		if err := resolveValueFileAndExternalRefsYamlNode(k, node, basePath); err != nil {
			return err
		}

		if mapNode, ok := node.(map[string]any); ok {
			if childNode, ok := mapNode["$file"]; ok {
				if file, ok := childNode.(string); ok {
					path := stdpath.Join(basePath, file)

					bytes, err := os.ReadFile(path)
					if err != nil {
						return fmt.Errorf("resolving $file relative to path %v failed. Cause: %v", basePath, err)
					}
					nodes[k] = string(bytes)
				} else {
					return errors.New(`$file field must have string value`)
				}
			}

			if childNode, ok := mapNode["$files"]; ok {
				if fileNodes, ok := childNode.([]any); ok {
					var result []string

					for _, fileNode := range fileNodes {
						if fileMapNode, ok := fileNode.(map[string]any); ok {
							if pathNode, ok := fileMapNode["path"]; ok {
								if path, ok := pathNode.(string); ok {
									absPath := stdpath.Join(basePath, path)

									bytes, err := os.ReadFile(absPath)
									if err != nil {
										return fmt.Errorf("resolving $files relative to path %v failed. Cause: %v", basePath, err)
									}
									result = append(result, string(bytes))
								} else {
									return errors.New(`path field in $files must have string value`)
								}
							} else if globNode, ok := fileMapNode["glob"]; ok {
								if glob, ok := globNode.(string); ok {
									absGlob := stdpath.Join(basePath, glob)

									matches, err := filepath.Glob(absGlob)
									if err != nil {
										return fmt.Errorf("resolving glob in $files relative to path %v failed. Cause: %v", basePath, err)
									}

									for _, match := range matches {
										bytes, err := os.ReadFile(match)
										if err != nil {
											return fmt.Errorf("resolving $files relative to path %v failed. Cause: %v", basePath, err)
										}
										result = append(result, string(bytes))
									}
								} else {
									return errors.New(`glob field in $files must have string value`)
								}
							} else {
								return errors.New(`each entry in $files must contain either a path field or a glob field`)
							}
						} else {
							return errors.New(`each entry in $files must be an object`)
						}
					}

					nodes[k] = result
				} else {
					return errors.New(`$files field must have array value`)
				}
			}

			if childNode, ok := mapNode["$ref"]; ok {
				if ref, ok := childNode.(string); ok {
					r, err := jsonreference.New(ref)
					if err != nil {
						return err
					}

					if r.HasFragmentOnly {
						continue // Local refs are resolved after loading complete structure
					}

					if r.HasFullURL && !r.HasFileScheme {
						return errors.New(`$ref field only supports local references and file references`)
					}

					if r.HasFileScheme || r.HasURLPathOnly {
						// Load the yaml file
						docPath := stdpath.Join(basePath, r.GetURL().Path)
						file, err := os.OpenFile(docPath, os.O_RDONLY, 0)
						if err != nil {
							return err
						}
						defer file.Close()

						var doc map[string]any
						decoder := yaml.NewDecoder(file, yaml.DisallowUnknownField())
						if err := decoder.Decode(&doc); err != nil {
							return fmt.Errorf("error decoding %v:\n%w", docPath, err)
						}

						if r.GetPointer().IsEmpty() {
							nodes[k] = doc

							continue
						}

						val, _, err := r.GetPointer().Get(doc)
						if err != nil {
							return err
						}
						nodes[k] = val

						continue
					}

				} else {
					return errors.New(`$ref field must have string value`)
				}
			}
		}
	}

	return nil
}

func resolveValueFileAndExternalRefsYamlNode(parent string, node any, path string) error {
	if mapNode, ok := node.(map[string]any); ok {
		return resolveValueFileAndExternalRefs(mapNode, path)
	}
	if sequenceNode, ok := node.([]any); ok {
		return resolveValueFileAndExternalRefsYamlSequence(parent, sequenceNode, path)
	}

	return nil
}

func resolveValueFileAndExternalRefsYamlSequence(parent string, node []any, path string) error {
	for _, v := range node {
		if err := resolveValueFileAndExternalRefsYamlNode(parent, v, path); err != nil {
			return err
		}
	}

	return nil
}
