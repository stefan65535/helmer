package domain

import (
	"errors"
	"fmt"

	"github.com/go-openapi/jsonreference"
	"github.com/goccy/go-yaml"
	"github.com/palantir/pkg/yamlpatch"
	"github.com/r3labs/diff/v3"
)

type Patch struct {
	Target        map[string]any  `yaml:"target"`
	PatchJSON6902 yamlpatch.Patch `yaml:"patch"` // Note: yamlpatch.Patch is a slice of yamlpatch.Operation.
}

func (p *Patch) Apply(manifest string, values map[string]any) (string, error) {
	var yamlManifest map[string]any

	err := yaml.Unmarshal([]byte(manifest), &yamlManifest)
	if err != nil {
		return "", err
	}

	matched, err := isFragmentInManifest(yamlManifest, p.Target)
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

		patchedManifest, err := yamlpatch.Apply([]byte(manifest), deReferencedPatch)
		if err != nil {
			return "", err
		}
		manifest = string(patchedManifest)

	}

	// TODO add warning if no match was found

	return manifest, nil
}

// isFragmentInManifest checks if the fragment exists in the document at root level.
func isFragmentInManifest(document map[string]any, fragment map[string]any) (bool, error) {
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
