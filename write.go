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
	needsLang := false
	for _, s := range p.Secrets {
		if len(s.Guide) > 0 {
			needsLang = true
			break
		}
	}
	if needsLang {
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
	targetSel := pkgName(targetPkg) + "." + targetIdent
	ciSel := pkgName(ciPkg) + "." + ciIdent
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
