package domain

import (
	"bytes"
	"errors"
	"text/template"

	"github.com/stefan65535/helmer/internal/logger"
)

func RenderTemplatesInValues(v map[string]any) (map[string]any, error) {
	b := map[string]any{}
	b["Values"] = v

	if err := renderTemplatesInValues("", b); err != nil {
		return nil, err
	}

	v, ok := b["Values"].(map[string]any)
	if !ok {
		return nil, errors.New("Values: must be a yaml object")
	}

	return v, nil
}

func renderTemplatesInValues(parent string, nodes map[string]any) error {
	for nodeName := range nodes {
		if err := renderTemplatesInValuesYamlNode(parent+"."+nodeName, nodes[nodeName]); err != nil {
			return err
		}

		if mapNode, ok := nodes[nodeName].(map[string]any); ok {
			if childNode, ok := mapNode["$template"]; ok {
				if len(mapNode) > 1 {
					return errors.New(`$template field must be the only field in the yaml object`)
				}
				if tpl, ok := childNode.(string); ok {
					logger.Verbosef(1, "%v.%v.$template: %v", parent, nodeName, tpl)

					tpl, err := template.New("tpl").Option("missingkey=error").Parse(tpl)
					if err != nil {
						return err
					}

					values := map[string]any{}
					values["Values"] = GlobalValues

					var buf bytes.Buffer
					err = tpl.Execute(&buf, values)
					if err != nil {
						return err
					}

					logger.Verbosef(2, "replaced with %v.%v: %v", parent, nodeName, buf.String())
					nodes[nodeName] = buf.String()
				} else {
					return errors.New(`$template field must have string type`)
				}
			}
		}
	}

	return nil
}

func renderTemplatesInValuesYamlNode(parent string, node any) error {
	if mapNode, ok := node.(map[string]any); ok {
		return renderTemplatesInValues(parent, mapNode)
	}
	if sequenceNode, ok := node.([]any); ok {
		return renderTemplatesInValuesYamlSequence(parent, sequenceNode)
	}

	return nil
}

func renderTemplatesInValuesYamlSequence(parent string, node []any) error {
	for _, v := range node {
		if err := renderTemplatesInValuesYamlNode(parent, v); err != nil {
			return err
		}
	}

	return nil
}
