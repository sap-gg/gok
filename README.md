# gok

`gok` is a command-line tool for rendering server configurations from layered templates. It is designed to help manage
configurations for multiple environments (e.g., development and production) by composing reusable template components.

## Core Concepts

* **Manifest (`gok-manifest.yaml`)**: The central entry point for the tool. This file defines one or more "targets" to
  be rendered.

* **Target**: A single, renderable output, such as a specific server instance. Each target has a unique name and is
  defined by a list of templates that are applied in order.

* **Template**: A directory containing a collection of files and configurations that represent a reusable component. For
  example, a base `paper` template could contain all the standard files for a PaperMC server.

* **Layers**: Templates are applied as layers. A target is built by applying a list of templates sequentially as defined
  in the manifest. Files from later templates will patch or overwrite files from earlier ones, allowing for specific
  overrides.

* **Values & Secrets**: The system separates non-sensitive configuration (`values`) from sensitive data (`secrets`).
  This data is injected into templates to produce the final output. `gok` is agnostic about secret management; it
  expects decrypted secrets to be provided at runtime.

## Features

* **Layered Templating**: Build configurations by composing smaller, reusable templates in a specific order.
* **Go Templating**: Files with a `.templ` infix (e.g., `config.yaml.templ`) are processed as Go templates, allowing for
  dynamic content generation.
* **Data Injection**: Provide non-sensitive values (`--values`) and sensitive secrets (`--secret-values`) to templates
  through a strictly-defined import system.
* **State Comparison (diff): Safely preview changes between a rendered artifact and a live environment,
  including conflict detection for manual changes.
* **Configuration Patching**: Automatically merges configuration files for YAML, JSON, TOML, and `.properties` formats,
  rather than overwriting them.
* **File Deletion**: Templates can explicitly delete files that were added by a previously applied template layer.
* **Archive & Directory Output**: The final rendered output can be saved as a directory, a `.tar` archive, or a
  compressed `.tar.gz` archive.

## Directory Structure

A typical `gok` project is structured with a central manifest that references templates and overlays located in
subdirectories.

```
.
├── gok-manifest.yaml
├── values/
│   ├── common.yaml
│   └── production.yaml
├── secrets/
│   └── production.sops.yaml  # (Intended to be decrypted before use)
├── templates/
│   └── paper/
│       ├── gok-template.yaml
│       ├── server.properties.templ
│       └── ...
└── overlays/
    └── survival-prod/
        ├── gok-template.yaml
        ├── gok-deletions.yaml
        └── server-icon.png
```

## Installation

To install `gok`, you can use `go install`:

```bash
go install github.com/sap-gg/gok@latest
```

---

## Usage

The tool is designed around a three-step workflow: **Render**, **Diff**, and **Apply**.

**1. Render:**

First, use `gok render` to process your manifest and templates, producing a versioned, self-contained artifact.

```bash
gok render -m <manifest-path> [target-selectors] -o <output-path> [value-files]
```

**2. Diff:**

Next, use `gok diff` to get a read-only preview of the changes that would be made by applying the artifact to a live
environment.
This command will detect any manual changes ("drift") on the target.

```bash
gok diff <source-artifact.tar.gz> <current-output-dir>
```

**3. Apply:**

Finally, use `gok apply` to apply the changes.
It will abort by default if it detects conflicts, protecting manual changes from being overwritten.

```bash
gok apply <source-artifact.tar.gz> --destination <dir>
```

---

### Examples

A typical workflow for deploying a change to a production server.

**Step 1: Render the production target with values and secrets into a versioned archive:**

```bash
# Decrypt secrets and pipe them to the render command
sops -d secrets/production.sops.yaml | gok render \
  -t survival-prod \
  -f values/common.yaml \
  -s - \
  -o survival-prod-v1.1.0.tar.gz
```

**Step 2: Diff the rendered artifact against the current live configuration:**

```bash
gok diff ./survival-prod-v1.1.0.tar.gz /opt/minecraft/survival
```

**Step 3: Apply the changes if the diff looks good:**

```bash
gok apply ./survival-prod-v1.1.0.tar.gz --destination /opt/minecraft/survival
```

---

## File Format Reference

### `(root)/gok-manifest.yaml`

This is the main entry point and defines all renderable targets.
Each target specifies an ordered list of templates to apply.

```yaml
version: 1

# Global, non-sensitive values available to all targets.
# These have the lowest precedence.
values:
  global_setting: "default"

# Defines all renderable outputs.
targets:
  # The key 'survival-prod' is the unique ID of the target.
  survival-prod:
    # Subdirectory within the output archive/directory for this target's files.
    output: "survival"
    tags:
      - "production"
      - "survival"
    # List of templates to apply, in order. The base template must be listed
    # explicitly, followed by any overlays or specializations.
    templates:
      - from: ./templates/paper
      - from: ./overlays/survival-prod
```

### `template/(root)/gok-template.yaml`

This optional file resides in a template's root directory to provide metadata and declare data dependencies.

```yaml
version: 1

name: "my-template"
description: "Provides the base configuration for a service."

maintainers:
  - name: "Team A"
    email: "team-a@example.com"

# Explicitly declare all data dependencies for this template.
# ONLY the declared imports will be available in templates.
imports:
  # Request non-sensitive values (from --values files).
  values:
    # Key uses dot-notation for nested access.
    "server.port":
      description: "The public port the server will listen on."
      required: true
    "server.motd":
      description: "The message of the day."
      default: "A default server message"

  # Request sensitive values (from --secret-values files).
  secrets:
    "database.password":
      description: "Password for the primary database connection."
      required: true

  # Request read-only access to the entire parsed gok-manifest.yaml.
  manifest:
    description: "Needed to read the output paths of other targets."
```

### `template/(root)/gok-deletions.yaml`

This optional file can be placed in a template or overlay directory
to specify files that should be deleted from the final output.

```yaml
version: 1

# List of file paths (relative to the template root) to delete.
# These files will be removed from the final output if they exist.
deletions:
  - path: "unwanted-file.txt"
  - path: plugins/
    recursive: true  # Delete plugins/ with all its contents
```

### `template/**/*.artifact.yaml`

These optional files can be placed anywhere within a template directory.
These files contain external assets that need to be fetched after rendering.

```yaml
version: 1

algorithm: "sha256"
checksum: "fefefefe...fefefefe"

source:
  http:
    url: "example.com/some-artifact.jar"
    headers:
      Authorization: "Basic dXNlcjpwYXNzd29yZA=="
```

---

## Templating

Files with a `.templ` infix are processed by Go's `text/template` engine.
(**note:** artifacts don't need to have the `.templ` infix)
The data passed to the template is structured
into scopes based on the `imports` declaration in the template's manifest.

**Example `config.yaml.templ`:**

```yaml
# Access data imported from --values files
port: { { .values.server.port } }
motd: "{{ .values.server.motd }}"

# Access data imported from --secret-values files
database:
  password: "{{ .secrets.database.password }}"

# Access data from the main manifest
# (only if enabled in gok-template.yaml)
otherTargets:
  proxyOutput: "{{ .manifest.Targets.proxy.Output }}"
```
