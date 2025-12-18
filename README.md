# Helmer

## Overview

Helmer is a tool for rendering sets of Helm Charts with values from multiple sources to multiple targets.

## Motivation

Have you enjoyed Kustomize’s ability to layer manifest changes to build multiple configurations from a small set of manifests, but found yourself getting lost in which configuration patches which resource, or feeling uncomfortable making changes to lower layers? Starting to miss your templates?

One of the main powers of a Helm chart and its templates is that it dictates what can be altered. It's not possible the user to change anything else than what has been chosen by the template creator. This might also be seen as a drawback, of course, as it means less freedom for the user. This is what is so appealing with Kustomize: the ability to change anything. However, the drawback with tools like Kustomize is fragility. If the author of the base manifest makes a change, it can easily break kustomizations that depends on finding an element at a certain place. This is especially cumbersome in arrays, as elements are referred to by their position. Change the order and the base YAML is still valid, but a patch will hit a different element than what it was intended for. In a small set of layers with a few patches this can be manageable, but in a larger set things can easily get complicates. Changing a base manifest can result in a lot of surprises.

Helm, on its side, has the limitation of using a single chart and single set of values. Sure, you can run Helm multiple times refer to multiple value sets, but you will have to manage them on your own. Feasible, but why not make a tool to aid in this process?

Enter Helmer. The intent is to utilize Helm to execute multiple charts and add layering of values in the form of separate configuration files that can include other configuration files to reuse or replace value sets and chart rendering directives.

## Installation

Helmer only has a compile-time dependency on Helm, so the resulting binary is self-contained and has no run-time dependencies on Helm, or any other tools for that matter.

### From source

```bash
go install github.com/stefan65535/helmer/cmd/helmer@latest
```

## Workflow

Helmer doesn't talk to your Kubernetes clusters. (At least not the current version. A future one might.) The intended workflow is instead to run the tool in a CI/CD pipeline and push the generated manifests to a second Git repo, the target. The Kubernetes clusters are expected to run a GitOps tool like ArgoCD that points to a directory in the target Git repo.

Having this two-step process might seem overcomplicated, but it has the benefit of we getting a chance to review not only the chart and value changes in the source repository but also to do a review of what will change and how in which cluster. This becomes more important when handling complex configurations for a large number of clusters.

This 'Git as the target' approach also lets us utilize the branching capabilities of Git. If you set up different branches for different environments in the target repo, you can have as fine-grained control as you like over which clusters get updated on a merge. Give each cluster its own branch or group them in into branches like sandbox, development, production to roll out big changes at a pace that suit your needs.

## Usage

Run
```helmer --help```

## Configuration file

### includes

The includes element contains a list of path references to other configuration files.

`path:` argument tells Helmer where to find the configuration to include. This path is relative to the current configuration file.

Included configuration files are limited to contain includes, charts, values, capabilities, and release directives.

### charts

A Chart references a Helm chart.

- `path`: Where to find the chart. Currently, only local charts are supported.
- `values`: Values to set for this chart. Any YAML valid in a Helm values.yaml file can be put here and will override the Charts built in defaults.

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

Values declare a set of global values that can be used for all charts. Any YAML valid as Helm values can be put here.

Example:

```yaml
values:
  Colour: Blue
```

##### $ref

A value node can contain a reference to another value through the $ref notation.
This is useful when working with third party charts where you can't change the value names in a chart but still want to use values already present in your value set. It also gives you the option to design your own charts that are unaware of your global values structure in the Helmer config and insteed name charts fields after there location or function in the chart.

The value of $ref uses the [JSON Pointer notation](https://tools.ietf.org/html/rfc6901). It is however limited to to URI fragments, i.e. it must start with a `#`.

References are resolved after all includes are processed. This lets you reference a field in a document anywher in the include tree.

Example:
Lets assume you hace a chart with a value field called FavouriteColor. You could introduce a value field with that name directly but perhaps tehre is already field in your Helmer config that plays the role of a favourite colour. Instead of changing your chart template a $ref in the FavouriteColor field will pick it upp for you.

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

A value node can contain a file reference by using the syntax $file.

```yaml
values:
  Colour:
    $file: ../favouriteColour.txt
```

This will convert the node to a scalar whose value is the content of the file.

Helm also has the ability to reference files from a chart. However, it is limited to files placed within the Chart. A Helmer $file reference on the other hand can reference any file on the host.

##### patches

Patches are applied to rendered Kubernetes manifests using JSON Patch (RFC 6902). Each patch entry must identify which rendered resources it should target and then provide a JSON Patch array of operations. Typical selection fields are:

- kind — the Kubernetes Kind (e.g., Deployment, Service)
- name — metadata.name of the resource
- namespace — metadata.namespace (optional)
- apiVersion — apiVersion of the resource (optional)
- labels — a map of labels to match (optional, matches resources that contain all specified label keys/values)

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

- The "patch" field is a JSON Patch array (list of objects with op, path, and optionally value).
- Paths are JSON Pointer strings (RFC 6901). Use ~1 to escape slashes and ~0 to escape tildes inside key names (for example, the annotation key "helm.sh/managed-by" becomes "helm.sh~1managed-by" in the pointer).
- Operations supported by RFC 6902 (add, remove, replace, move, copy, test) are accepted.
- Values in a patch operation can be scalars, objects, arrays, or special Helmer references ($ref), see below.

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

4) Reuse a value from your configuration via $ref

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
- If a replace/remove operation targets a missing path, the patch will fail. Use add to create missing fields, but be aware add inserts into arrays by index and will not implicitly create intermediate objects.
- For complex structural changes (especially in arrays) prefer adjusting Helm values or adding a new rendered manifest instead of fragile index-based patches.

Error handling

- If a patch fails (invalid path or operation), Helmer will surface an error for that chart/rendering step. Test patches locally against rendered output to validate pointer paths and operations before adding them to a pipeline.

This should give you the tools to target individual rendered resources and apply precise JSON Patch edits while still using $ref to keep patches data-driven and reusable.

#### capabilities

This provides information about what capabilities the Kubernetes cluster supports.

As Helmer doesn't talk to your Kubernetes cluster, Helmer can't extract information about the cluster by itself. Often this is not a problem when running helm on the client side, but in th rare case you are using charts that reference the built-in object Capabilities, its values can be set explicitly through this directive.

- `apiVersions:` Sets the Capabilities.APIVersions.
- `kubeVersion.version:` Sets the Kubernetes Capabilities.KubeVersion.Version.
- `kubeVersion.major:` Sets the Capabilities.KubeVersion.Major.
- `kubeVersion.minor:` Sets the Capabilities.KubeVersion.Minor.

#### release

Sets various attributes in the Helm built-in object `.Release`.

- `namespace:` Sets the .Release.Namespace
- `name:` Sets the .Release.Name

Example:

```yaml
release:
  namespace: mynamespace
```

### target

A target directive controls the generation of manifests from the defined set of charts and Helm objects in the configuration.

- `path:` tells Helmer where to put the generated manifests.

Example:

```yaml
target:
  path: write/manifests/to/this/file
```

## Priority order for values

Values can be set as globals or in a chart. There are also the built-in value defaults in Helm charts themselves. Priority among these is: Chart > Globals > Chart defaults. Global values included from another configuration will have lower priority than those in the including configuration.

# License

Helmer is released under the Apache 2.0 license. See [LICENSE](LICENSE)
