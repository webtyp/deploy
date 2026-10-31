---
PLAN: "feat: deploy.Write serializes Plan into config/deploy.go"
TAG: v0.2.115
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

## Tools

| Tool | Install |
|---|---|
| `gotest` | `go install webtyp.com/devflow/cmd/gotest@latest` |

## Ecosystem search

1. Existing code: `webtyp.com/deploy` exports `Scan(rootDir string) (Plan, error)` in `scan.go`, which parses `config/deploy.go` using `go/parser` and AST.
2. What is missing: The inverse operation `Write(rootDir string, p Plan) error`, needed by `webtyp/app`'s guided deployment wizard to scaffold or auto-create `config/deploy.go` when a project lacks one.
3. Decision: **extend** `webtyp.com/deploy` with `Write`.

## Design gate

1. **Prior art:**
   - Terraform / Pulumi: generates configuration file or declaration from state.
   - [Fly.io CLI (`fly launch`)](https://fly.io/docs/flyctl/launch/): inspects source code, generates canonical `fly.toml`.
   - Go `ast.Fprint` / `go/format`: formats Go source code AST into valid files.
2. **Novice-name test:** `deploy.Write(rootDir, plan)` is the exact symmetrical verb to `deploy.Scan(rootDir)`. A junior developer understands its purpose without opening the documentation.
3. **Complexity ledger:** Zero new external dependencies. Standard library `os`, `path/filepath`, `strings`, `fmt`, `go/format`.
4. **Where it belongs:** `webtyp.com/deploy` in `write.go`.
5. **What it deletes:** Eliminates manual boilerplate editing of `config/deploy.go` for new projects in `webtyp/app`.

## Stages

### Stage 1: Implement `deploy.Write` in `write.go`

Create `write.go` with:
- `func Write(rootDir string, p Plan) error`:
  1. Calls `validate(p)`. If invalid, returns the validation error.
  2. Resolves imports from `p.Target` and `p.CI`. A `TargetID` or `CIID` has the format `<import_path>.<ident>` (e.g. `webtyp.com/selfupdate.Server` or `webtyp.com/goflare.Worker`, and `webtyp.com/gorelease.Local` or `webtyp.com/github.Actions`).
  3. Prepares the formatted Go source code for `config/deploy.go`:
     - Starts with header: `//go:build deploy\n\n`
     - Package declaration: `package config\n\n`
     - Imports `webtyp.com/deploy`, the target's import path, the CI's import path, and `webtyp.com/lang` (if `p.Secrets` has entries).
     - Function `func Deploy() deploy.Plan`: returns literal `deploy.Plan{...}` with all fields from `p` (`Name`, `Domain`, `Target`, `CI`, `Databases`, `Buckets`, `Vars`, `Secrets`).
  4. Runs `go/format.Source` on the generated Go code.
  5. Atomically writes the file to `filepath.Join(rootDir, DefaultFile)` (creating directory `filepath.Join(rootDir, "config")` with mode 0755 if it does not exist). Atomic write: write to `.tmp` file with mode 0644, then rename.

Verbatim structure of `write.go`:

```go
package deploy

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

// Write writes p as config/deploy.go in rootDir, formatted with go/format.
// It creates the config/ directory if missing and writes atomically.
// Example: err := deploy.Write(".", plan)
func Write(rootDir string, p Plan) error {
	if err := validate(p); err != nil {
		return err
	}
	targetPkg, targetIdent, err := splitID(string(p.Target))
	if err != nil {
		return fmt.Errorf("%w: target %s", ErrTargetMismatch, p.Target)
	}
	ciPkg, ciIdent, err := splitID(string(p.CI))
	if err != nil {
		return fmt.Errorf("%w: CI %s", ErrCIMismatch, p.CI)
	}

	imports := []string{`"webtyp.com/deploy"`}
	if targetPkg != "webtyp.com/deploy" {
		imports = append(imports, fmt.Sprintf("%q", targetPkg))
	}
	if ciPkg != "webtyp.com/deploy" && ciPkg != targetPkg {
		imports = append(imports, fmt.Sprintf("%q", ciPkg))
	}
	if len(p.Secrets) > 0 {
		imports = append(imports, `"webtyp.com/lang"`)
	}

	var buf bytes.Buffer
	buf.WriteString("//go:build deploy\n\npackage config\n\nimport (\n")
	for _, imp := range imports {
		buf.WriteString("\t" + imp + "\n")
	}
	buf.WriteString(")\n\nfunc Deploy() deploy.Plan {\n\treturn deploy.Plan{\n")
	buf.WriteString(fmt.Sprintf("\t\tName: %q,\n", p.Name))
	if p.Domain != "" {
		buf.WriteString(fmt.Sprintf("\t\tDomain: %q,\n", p.Domain))
	}
	targetSel := targetIdent
	if targetPkg != "webtyp.com/deploy" {
		targetSel = pkgName(targetPkg) + "." + targetIdent
	}
	ciSel := ciIdent
	if ciPkg != "webtyp.com/deploy" {
		ciSel = pkgName(ciPkg) + "." + ciIdent
	}
	buf.WriteString(fmt.Sprintf("\t\tTarget: %s,\n", targetSel))
	buf.WriteString(fmt.Sprintf("\t\tCI: %s,\n", ciSel))

	if len(p.Databases) > 0 {
		buf.WriteString("\t\tDatabases: []deploy.Database{\n")
		for _, db := range p.Databases {
			buf.WriteString(fmt.Sprintf("\t\t\t{Binding: %q, Name: %q},\n", db.Binding, db.Name))
		}
		buf.WriteString("\t\t},\n")
	}
	if len(p.Buckets) > 0 {
		buf.WriteString("\t\tBuckets: []deploy.Bucket{\n")
		for _, b := range p.Buckets {
			buf.WriteString(fmt.Sprintf("\t\t\t{Binding: %q, Name: %q},\n", b.Binding, b.Name))
		}
		buf.WriteString("\t\t},\n")
	}
	if len(p.Vars) > 0 {
		buf.WriteString("\t\tVars: []deploy.Var{\n")
		for _, v := range p.Vars {
			buf.WriteString(fmt.Sprintf("\t\t\t{Key: %q, Value: %q},\n", v.Key, v.Value))
		}
		buf.WriteString("\t\t},\n")
	}
	if len(p.Secrets) > 0 {
		buf.WriteString("\t\tSecrets: []deploy.Secret{\n")
		for _, s := range p.Secrets {
			buf.WriteString("\t\t\t{\n")
			buf.WriteString(fmt.Sprintf("\t\t\t\tKey: %q,\n", s.Key))
			buf.WriteString(fmt.Sprintf("\t\t\t\tWhat: %q,\n", string(s.What)))
			sourceStr := "deploy.Typed"
			if s.Source == Random {
				sourceStr = "deploy.Random"
			}
			buf.WriteString(fmt.Sprintf("\t\t\t\tSource: %s,\n", sourceStr))
			if len(s.Guide) > 0 {
				buf.WriteString("\t\t\t\tGuide: []lang.Text{\n")
				for _, g := range s.Guide {
					buf.WriteString(fmt.Sprintf("\t\t\t\t\t%q,\n", string(g)))
				}
				buf.WriteString("\t\t\t\t},\n")
			}
			if len(s.To) > 0 {
				buf.WriteString("\t\t\t\tTo: []deploy.Place{")
				var places []string
				for _, pl := range s.To {
					switch pl {
					case ToRuntime:
						places = append(places, "deploy.ToRuntime")
					case ToCI:
						places = append(places, "deploy.ToCI")
					case ToLocal:
						places = append(places, "deploy.ToLocal")
					}
				}
				buf.WriteString(strings.Join(places, ", "))
				buf.WriteString("},\n")
			}
			buf.WriteString("\t\t\t},\n")
		}
		buf.WriteString("\t\t},\n")
	}

	buf.WriteString("\t}\n}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("deploy.Write formatting: %w\nsource:\n%s", err, buf.String())
	}

	configDir := filepath.Join(rootDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	destPath := filepath.Join(rootDir, DefaultFile)
	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, formatted, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, destPath)
}

func splitID(id string) (pkg, ident string, err error) {
	lastDot := strings.LastIndex(id, ".")
	if lastDot == -1 {
		return "", "", fmt.Errorf("invalid ID: %s", id)
	}
	return id[:lastDot], id[lastDot+1:], nil
}

func pkgName(importPath string) string {
	lastSlash := strings.LastIndex(importPath, "/")
	if lastSlash == -1 {
		return importPath
	}
	return importPath[lastSlash+1:]
}
```

### Stage 2: Tests in `tests/write_test.go`

Create `tests/write_test.go` verifying:
1. `TestWrite_Validation`: Invalid Plan (empty Name, empty Target, etc.) fails before creating files.
2. `TestWrite_RoundTrip`: Writing a Plan with `Write`, then reading it back with `Scan`, yields an identical `Plan`. Test with:
   - Worker + GitHub Actions (`webtyp.com/goflare.Worker`, `webtyp.com/github.Actions`) with Databases and Buckets.
   - Selfhost + Gorelease (`webtyp.com/selfupdate.Server`, `webtyp.com/gorelease.Local`) with Secrets and Vars.

Verbatim test code:

```go
package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/lang"
)

func TestWrite_Validation(t *testing.T) {
	tmp := t.TempDir()
	err := deploy.Write(tmp, deploy.Plan{})
	if err == nil {
		t.Fatal("expected error on empty plan")
	}
}

func TestWrite_RoundTrip(t *testing.T) {
	plans := []deploy.Plan{
		{
			Name:      "shop",
			Domain:    "app.example.com",
			Target:    "webtyp.com/selfupdate.Server",
			CI:        "webtyp.com/gorelease.Local",
			Vars:      []deploy.Var{{Key: "APP_ENV", Value: "production"}},
			Secrets: []deploy.Secret{
				{
					Key:    "TOKEN",
					What:   "API token",
					Source: deploy.Typed,
					Guide:  []lang.Text{"Go to dashboard", "Create token"},
					To:     []deploy.Place{deploy.ToRuntime},
				},
				{
					Key:    "KEY",
					What:   "Random key",
					Source: deploy.Random,
					To:     []deploy.Place{deploy.ToRuntime, deploy.ToLocal},
				},
			},
		},
		{
			Name:      "worker-app",
			Domain:    "worker.example.com",
			Target:    "webtyp.com/goflare.Worker",
			CI:        "webtyp.com/github.Actions",
			Databases: []deploy.Database{{Binding: "DB", Name: "my-db"}},
			Buckets:   []deploy.Bucket{{Binding: "UPLOADS", Name: "my-bucket"}},
		},
	}

	for _, p := range plans {
		t.Run(p.Name, func(t *testing.T) {
			tmp := t.TempDir()
			if err := deploy.Write(tmp, p); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			// Read back using deploy.Scan
			scanned, err := deploy.Scan(tmp)
			if err != nil {
				content, _ := os.ReadFile(filepath.Join(tmp, deploy.DefaultFile))
				t.Fatalf("Scan failed: %v\nFile content:\n%s", err, string(content))
			}

			if !reflect.DeepEqual(scanned, p) {
				t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", scanned, p)
			}
		})
	}
}
```

### Stage 3: Documentation in `README.md`

Update `README.md` to document `deploy.Write` alongside `deploy.Scan`, following the concise 4-section documentation standard (≤ 150 lines):
1. `## Why This Exists`: mention `deploy.Scan` and `deploy.Write`.
2. `## Before You Start`: prerequisites.
3. `## Step-by-Step Guide`: complete compilable Go example generating and scanning a deployment plan.
4. `## Documentation Map`.

## Delivery check

1. `gotest` passes cleanly:
   ```bash
   GOPRIVATE=webtyp.com/*,github.com/veltylabs/* GOFLAGS=-mod=mod gotest
   ```
2. Commit and open PR.
