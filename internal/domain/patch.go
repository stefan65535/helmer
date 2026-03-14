package domain

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/go-openapi/jsonreference"
	"github.com/goccy/go-yaml"
	"github.com/palantir/pkg/yamlpatch"
	"github.com/r3labs/diff/v3"
)

type Patch struct {
	Target        PatchTarget     `yaml:"target"`
	PatchJSON6902 yamlpatch.Patch `yaml:"patch"` // Note: yamlpatch.Patch is a slice of yamlpatch.Operation.
}

type PatchTarget struct {
	APIVersion string `yaml:"apiVersion,omitempty"`
	Group      string `yaml:"group,omitempty"`
	Version    string `yaml:"version,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Name       string `yaml:"name,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
}

func (p *Patch) Apply(manifests string, values map[string]any) (string, error) {
	result := bytes.NewBuffer(make([]byte, 0, len(manifests)))

	docs := splitYAMLDocuments(manifests)
	for _, doc := range docs {
		result.WriteString("---\n")

		var yamlManifest map[string]any
		err := yaml.Unmarshal(doc, &yamlManifest)
		if err != nil {
			return "", err
		}

		matched, err := isTargetInManifest(yamlManifest, p.Target)
		if err != nil {
			return "", err
		}

		if matched {
			var deReferencedPatch yamlpatch.Patch

			for _, operation := range p.PatchJSON6902 {
				if mapNode, ok := operation.Value.(map[string]any); ok {
					if childNode, ok := mapNode["$ref"]; ok {
						if ref, ok := childNode.(string); ok {
							r, err := jsonreference.New(ref)
							if err != nil {
								return "", err
							}

							if r.HasFragmentOnly {
								val, _, err := r.GetPointer().Get(values)
								if err != nil {
									return "", fmt.Errorf(`error evaluating reference "%v": %v`, ref, err)
								}

								deReferencedOperation := yamlpatch.Operation{
									Type:    operation.Type,
									Path:    operation.Path,
									From:    operation.From,
									Value:   val,
									Comment: operation.Comment,
								}

								deReferencedPatch = append(deReferencedPatch, deReferencedOperation)
							} else {
								return "", errors.New(`$ref field in patch only supports /# style references`)
							}
						} else {
							return "", errors.New(`$ref field must have string type`)
						}
					}
				} else {
					deReferencedPatch = append(deReferencedPatch, operation)
				}
			}

			patchedDoc, err := yamlpatch.Apply(doc, deReferencedPatch)
			if err != nil {
				return "", err
			}

			result.Write(patchedDoc)
		} else {
			result.Write(doc)
		}
	}

	// TODO check expected target value exists in the patched document and throw an error if not. This is to prevent silent failures where the patch is applied to the wrong document because the target selector is too broad or the template has changes unexpectedly.

	return result.String(), nil
}

// isTargetInManifest checks if the fragment exists in the document at root level.
func isTargetInManifest(document map[string]any, target PatchTarget) (bool, error) {
	fragment := make(map[string]any)
	if target.Group != "" {
		groupVersion := target.Group
		if target.Version != "" {
			groupVersion += "/" + target.Version
		}
		fragment["apiVersion"] = groupVersion
	} else if target.Version != "" {
		fragment["apiVersion"] = target.Version
	}
	
	if target.APIVersion != "" {
		fragment["apiVersion"] = target.APIVersion
	}

	if target.Kind != "" {
		fragment["kind"] = target.Kind
	}

	if target.Name != "" || target.Namespace != "" {
		metadata := make(map[string]any)
		if target.Name != "" {
			metadata["name"] = target.Name
		}
		if target.Namespace != "" {
			metadata["namespace"] = target.Namespace
		}
		fragment["metadata"] = metadata
	}

	changelog, err := diff.Diff(fragment, document)
	if err != nil {
		return false, err
	}

	// The diff changelog contains all changes nedded to mutate the fragment to a full document.
	// If the fragment exists within the document the changelog will only contain actions to create and no updates or deletions.
	for _, change := range changelog {
		if change.Type == diff.UPDATE || change.Type == diff.DELETE {
			return false, nil
		}
	}

	return true, nil
}

// splitYAMLDocuments splits a YAML string into separate documents based on the document separator "---".
func splitYAMLDocuments(yamlContent string) [][]byte {
	var docs [][]byte

	lines := strings.SplitSeq(yamlContent, "\n")
	for line := range lines {
		if line == "---" {
			docs = append(docs, []byte{})
		} else {
			if len(docs) == 0 {
				docs = append(docs, []byte{})
			}
			docs[len(docs)-1] = append(docs[len(docs)-1], []byte(line+"\n")...)
		}
	}

	return docs
}
