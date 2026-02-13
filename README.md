# Helmer

## Overview

Helmer is a tool for rendering sets of Helm charts with values from multiple sources to multiple targets.

## Motivation

Have you enjoyed Kustomize’s ability to layer manifest changes to produce multiple configurations from a small set of manifests, but found yourself getting lost about which configuration patches which resource or felt uncomfortable making changes to lower layers? Missing your templates?

One of the main strengths of a Helm chart and its templates is that they dictate what can be altered. Users can't change anything other than what the template author exposes. That can be a drawback because it reduces flexibility. That's what makes Kustomize appealing: the ability to change anything. However, tools like Kustomize can be fragile. If the author of the base manifest makes a change, it can easily break kustomizations that depend on finding an element at a certain place. This is especially cumbersome in arrays, where elements are referred to by their position. Change the order and the base YAML is still valid, but a patch may hit a different element than intended. In a small set of layers with a few patches this can be manageable, but in a larger set things can easily get complicated. Changing a base manifest can result in many surprises.

Helm, on the other hand, is typically used with a single chart and a single set of values. You can run Helm multiple times to reference multiple value sets, but you'll have to manage them yourself. That's feasible, but why not make a tool to help with this?

Enter Helmer. Helmer aims to use Helm to execute multiple charts and add layering of values via separate configuration files. Those configuration files can include other files to reuse or replace value sets and chart rendering directives.

## Installation

Helmer has only a compile-time dependency on Helm; the resulting binary is self-contained and has no runtime dependency on Helm or any other tools.

### From source

```bash
go install github.com/stefan65535/helmer/cmd/helmer@latest
```

## Workflow

Helmer doesn't communicate with your Kubernetes clusters (at least not in the current version; a future version might). The intended workflow is to run the tool in a CI/CD pipeline and push the generated manifests to a second Git repository, the target. The Kubernetes clusters are expected to run a GitOps tool like ArgoCD that points to a directory in the target Git repository.

This two-step process might seem overcomplicated, but it gives you a chance to review not only the chart and value changes in the source repository but also to review what will change in each cluster. This becomes more important when handling complex configurations across many clusters.

This "Git as the target" approach also lets you use Git's branching capabilities. If you set up different branches for different environments in the target repo, you can control which clusters get updated on a merge. You can give each cluster its own branch or group them into branches like sandbox, development, and production to roll out changes at a pace that suits your needs.

## Usage

Run:

```bash
helmer --help
```

## Configuration file

### includes

The `includes` element contains a list of path references to other configuration files.

The `path` argument tells Helmer where to find the configuration to include. The path is relative to the current configuration file.

Included configuration files may contain only includes, charts, values, capabilities, and release directives. Targets are not allowed.

### charts

A chart references a Helm chart.

- `path`: Where to find the chart. Currently, only local charts are supported.
- `values`: Values to set for this chart. Any YAML valid in a Helm values.yaml file can be placed here and will override the chart's built-in defaults.

Example:

```yaml
charts:
  - path: path/to/my/helm/chart
    values:
      Colour: Yellow
```

### Helm objects

Helm objects correspond to the Helm built-in objects as described by [Helm Built-in Objects](https://helm.sh/docs/chart_template_guide/builtin_objects)

#### values

Values declare a set of global values that can be used for all charts. Any YAML valid as Helm values can be placed here.

Example:

```yaml
values:
  Colour: Blue
```

##### $ref

A value node can contain a reference to another value using the `$ref` notation. This is useful when working with third-party charts where you can't change the value names in the chart but still want to reuse values already present in your value set. It also lets you design charts that are unaware of your global values structure in the Helmer config and instead name chart fields after their location or function.

The value of `$ref` uses the [JSON Pointer notation](https://tools.ietf.org/html/rfc6901). It is limited to URI fragments, i.e., it must start with `#`.

References are resolved after all includes are processed. This lets you reference a field anywhere in the include tree.

Example:

Let's assume you have a chart with a value field called `FavouriteColor`. You could introduce a value field with that name directly, but perhaps there is already a field in your Helmer config that plays the role of a favourite colour. Instead of changing your chart template, a `$ref` in the `FavouriteColor` field will pick it up for you.

```yaml
charts:
  - path: "../../charts/mychart"
    values:
      FavouriteColor:
        $ref: "#/Colour"
```

```yaml
values:
  Colour: Blue
```

##### $file reference

A value node can contain a file reference by using the `$file` syntax.

```yaml
values:
  Colour:
    $file: ../favouriteColour.txt
```

This will convert the node to a scalar whose value is the content of the file.

Helm can reference files from a chart, but it is limited to files placed within the chart. A Helmer `$file` reference, on the other hand, can reference any file on the host.

##### patches

Patches are applied to rendered Kubernetes manifests using JSON Patch (RFC 6902). Each patch entry must identify which rendered resources it should target and then provide a JSON Patch array of operations. Typical selection fields are:

- `kind` — the Kubernetes Kind (e.g., Deployment, Service)
- `name` — metadata.name of the resource
- `namespace` — metadata.namespace (optional)
- `apiVersion` — apiVersion of the resource (optional)
- `labels` — a map of labels to match (optional; matches resources that contain all specified label keys/values)

A patch entry that matches multiple resources will be applied to each matching resource.

Patch structure

A patch entry under a chart looks like:

```yaml
charts:
  - path: charts/myapp
    patches:
      - target:
          kind: Deployment
          name: myapp
        patch:
          - op: replace
            path: /spec/template/spec/containers/0/image
            value: myrepo/myapp:1.2
```

Notes:

- The `patch` field is a JSON Patch array (a list of objects with `op`, `path`, and optionally `value`).
- Paths are JSON Pointer strings (RFC 6901). Use `~1` to escape slashes and `~0` to escape tildes inside key names (for example, the annotation key `helm.sh/managed-by` becomes `helm.sh~1managed-by` in the pointer).
- Operations supported by RFC 6902 (`add`, `remove`, `replace`, `move`, `copy`, `test`) are accepted.
- Values in a patch operation can be scalars, objects, arrays, or Helmer references (`$ref`).

Examples

1) Replace container image

```yaml
- target:
    kind: Deployment
    name: myapp
  patch:
    - op: replace
      path: /spec/template/spec/containers/0/image
      value: myrepo/myapp:1.2.3
```

2) Add an annotation (note escaped slash)

```yaml
- target:
    kind: Service
    name: my-service
  patch:
    - op: add
      path: /metadata/annotations/helm.sh~1managed-by
      value: helmer
```

3) Reuse a value from your configuration via `$ref`

```yaml
values:
  image: "myrepo/myapp:1.2.3"

charts:
  - path: charts/myapp
    patches:
      - selection:
          kind: Deployment
          name: myapp
        patch:
          - op: replace
            path: /spec/template/spec/containers/0/image
            value:
              $ref: values.image # resolves to "myrepo/myapp:1.2.3"
```

Behavior and best practices

- Patches are applied after Helm template rendering and before writing manifests to the target.
- Patches are applied in the order they appear. If multiple patches target the same path, later patches overwrite earlier ones.
- JSON Pointer array indices are position-based; using array indices can be fragile if upstream charts reorder array elements. When possible, prefer changing chart values or replacing larger subtrees instead of relying on numeric array indices.
- If a `replace`/`remove` operation targets a missing path, the patch will fail. 

Error handling

- If a patch fails (invalid path or operation), Helmer will surface an error for that chart/rendering step. Test patches locally against rendered output to validate pointer paths and operations before adding them to a pipeline.

This should give you the tools to target individual rendered resources and apply precise JSON Patch edits while still using `$ref` to keep patches data-driven and reusable.

#### capabilities

This provides information about what capabilities the Kubernetes cluster supports.

As Helmer doesn't talk to your Kubernetes cluster, it can't extract cluster information by itself. Often this is not a problem when running Helm on the client side, but in the rare case you are using charts that reference the built-in object `Capabilities`, its values can be set explicitly through this directive.

- `apiVersions:` Sets the `Capabilities.APIVersions`.
- `kubeVersion.version:` Sets the Kubernetes `Capabilities.KubeVersion.Version`.
- `kubeVersion.major:` Sets the `Capabilities.KubeVersion.Major`.
- `kubeVersion.minor:` Sets the `Capabilities.KubeVersion.Minor`.

#### release

Sets various attributes in the Helm built-in object `.Release`.

Theese attributes are not used by Helmer. It is only necessary to define these if your Helm charts use them.

- `namespace:` Sets the `.Release.Namespace`.
- `name:` Sets the `.Release.Name`.

Example:

```yaml
release:
  namespace: mynamespace
```

release can be set as a global directive and on chart basis. If both are set the chart will override the global value.

### target

A `target` directive controls the generation of manifests from the defined set of charts and Helm objects in the configuration.

- `path:` tells Helmer where to put the generated manifests.

Example:

```yaml
target:
  path: write/manifests/to/this/file
```

## Helmer values

Helmer values are added to the global .Values in Helm under .Values.Helmer
Currently only target.path is available
Example:

Some chart:

```yaml
... som k8s kind
spec:
  path: {{ .Values.Helmer.target.path }}
```

This can be useful in, for example, en ArgoCD applications that needs to reference the target path in a repository with freshly generated manifests.

## Priority order for values

Values can be set as globals or in a chart. There are also the built-in value defaults in Helm charts themselves. Priority among these is: Chart > Globals > Chart defaults. Global values included from another configuration will have lower priority than those in the including configuration.

# License

Helmer is released under the Apache 2.0 license. See [LICENSE](LICENSE)
