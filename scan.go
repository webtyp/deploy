package deploy

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"webtyp.com/lang"
)

// DefaultFile is where a project declares its deployment.
const DefaultFile = "config/deploy.go"

// Scan reads rootDir/DefaultFile and returns the Plan its Deploy function
// returns. Only literals are allowed; anything else is an error naming the
// file and line. A missing file is ErrNoPlan.
func Scan(rootDir string) (Plan, error) {
	path := filepath.Join(rootDir, DefaultFile)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return Plan{}, ErrNoPlan
	}

	// Check build tag
	hasTag := false
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.TrimSpace(c.Text) == "//go:build deploy" {
				hasTag = true
				break
			}
		}
		if hasTag {
			break
		}
	}
	if !hasTag {
		return Plan{}, ErrMissingBuildTag
	}

	var plan Plan
	var scanErr error

	ast.Inspect(f, func(n ast.Node) bool {
		if scanErr != nil {
			return false
		}

		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Deploy" {
			return true
		}

		if len(funcDecl.Body.List) != 1 {
			scanErr = fmt.Errorf("%s:%d: %w", path, fset.Position(funcDecl.Pos()).Line, ErrOnlyLiterals)
			return false
		}
		retStmt, ok := funcDecl.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(retStmt.Results) != 1 {
			scanErr = fmt.Errorf("%s:%d: %w", path, fset.Position(funcDecl.Pos()).Line, ErrOnlyLiterals)
			return false
		}
		compLit, ok := retStmt.Results[0].(*ast.CompositeLit)
		if !ok {
			scanErr = fmt.Errorf("%s:%d: %w", path, fset.Position(retStmt.Results[0].Pos()).Line, ErrOnlyLiterals)
			return false
		}

		plan, scanErr = parsePlanLit(fset, path, f.Imports, compLit)
		return false
	})

	if scanErr != nil {
		return Plan{}, scanErr
	}
	if err := validate(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func parsePlanLit(fset *token.FileSet, path string, imports []*ast.ImportSpec, lit *ast.CompositeLit) (Plan, error) {
	var p Plan
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return p, fmt.Errorf("%s:%d: %w", path, fset.Position(elt.Pos()).Line, ErrOnlyLiterals)
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			return p, fmt.Errorf("%s:%d: %w", path, fset.Position(kv.Key.Pos()).Line, ErrOnlyLiterals)
		}
		switch ident.Name {
		case "Name":
			p.Name = extractString(kv.Value)
		case "Domain":
			p.Domain = extractString(kv.Value)
		case "Target":
			val, err := extractSelector(fset, path, imports, kv.Value)
			if err != nil {
				return p, err
			}
			p.Target = TargetID(val)
		case "CI":
			val, err := extractSelector(fset, path, imports, kv.Value)
			if err != nil {
				return p, err
			}
			p.CI = CIID(val)
		case "Databases":
			dbs, err := parseDatabases(fset, path, kv.Value)
			if err != nil {
				return p, err
			}
			p.Databases = dbs
		case "Buckets":
			buckets, err := parseBuckets(fset, path, kv.Value)
			if err != nil {
				return p, err
			}
			p.Buckets = buckets
		case "Vars":
			vars, err := parseVars(fset, path, kv.Value)
			if err != nil {
				return p, err
			}
			p.Vars = vars
		case "Secrets":
			secrets, err := parseSecrets(fset, path, kv.Value)
			if err != nil {
				return p, err
			}
			p.Secrets = secrets
		default:
			return p, fmt.Errorf("%s:%d: %w", path, fset.Position(ident.Pos()).Line, ErrUnknownField)
		}
	}
	return p, nil
}

func parseDatabases(fset *token.FileSet, path string, expr ast.Expr) ([]Database, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(expr.Pos()).Line, ErrOnlyLiterals)
	}
	var dbs []Database
	for _, elt := range lit.Elts {
		clit, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(elt.Pos()).Line, ErrOnlyLiterals)
		}
		var db Database
		for _, kvelt := range clit.Elts {
			kv, ok := kvelt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ident, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Name == "Binding" {
				db.Binding = extractString(kv.Value)
			} else if ident.Name == "Name" {
				db.Name = extractString(kv.Value)
			} else {
				return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(ident.Pos()).Line, ErrUnknownField)
			}
		}
		dbs = append(dbs, db)
	}
	return dbs, nil
}

func parseBuckets(fset *token.FileSet, path string, expr ast.Expr) ([]Bucket, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(expr.Pos()).Line, ErrOnlyLiterals)
	}
	var buckets []Bucket
	for _, elt := range lit.Elts {
		clit, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(elt.Pos()).Line, ErrOnlyLiterals)
		}
		var b Bucket
		for _, kvelt := range clit.Elts {
			kv, ok := kvelt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ident, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Name == "Binding" {
				b.Binding = extractString(kv.Value)
			} else if ident.Name == "Name" {
				b.Name = extractString(kv.Value)
			} else {
				return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(ident.Pos()).Line, ErrUnknownField)
			}
		}
		buckets = append(buckets, b)
	}
	return buckets, nil
}

func parseVars(fset *token.FileSet, path string, expr ast.Expr) ([]Var, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(expr.Pos()).Line, ErrOnlyLiterals)
	}
	var vars []Var
	for _, elt := range lit.Elts {
		clit, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(elt.Pos()).Line, ErrOnlyLiterals)
		}
		var v Var
		for _, kvelt := range clit.Elts {
			kv, ok := kvelt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ident, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Name == "Key" {
				v.Key = extractString(kv.Value)
			} else if ident.Name == "Value" {
				v.Value = extractString(kv.Value)
			} else {
				return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(ident.Pos()).Line, ErrUnknownField)
			}
		}
		vars = append(vars, v)
	}
	return vars, nil
}

func parseSecrets(fset *token.FileSet, path string, expr ast.Expr) ([]Secret, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(expr.Pos()).Line, ErrOnlyLiterals)
	}
	var secrets []Secret
	for _, elt := range lit.Elts {
		clit, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(elt.Pos()).Line, ErrOnlyLiterals)
		}
		var s Secret
		for _, kvelt := range clit.Elts {
			kv, ok := kvelt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ident, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Name == "Key" {
				s.Key = extractString(kv.Value)
			} else if ident.Name == "What" {
				s.What = lang.Text(extractText(kv.Value))
			} else if ident.Name == "Source" {
				sel, ok := kv.Value.(*ast.SelectorExpr)
				if !ok {
					// could be an ident directly if they use import .
					id, ok := kv.Value.(*ast.Ident)
					if ok && id.Name == "Typed" {
						s.Source = Typed
					} else if ok && id.Name == "Random" {
						s.Source = Random
					} else {
						return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(kv.Value.Pos()).Line, ErrOnlyLiterals)
					}
				} else {
					if sel.Sel.Name == "Typed" {
						s.Source = Typed
					} else if sel.Sel.Name == "Random" {
						s.Source = Random
					} else {
						return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(sel.Pos()).Line, ErrOnlyLiterals)
					}
				}
			} else if ident.Name == "Guide" {
				gLit, ok := kv.Value.(*ast.CompositeLit)
				if !ok {
					return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(kv.Value.Pos()).Line, ErrOnlyLiterals)
				}
				for _, gelt := range gLit.Elts {
					s.Guide = append(s.Guide, lang.Text(extractText(gelt)))
				}
			} else if ident.Name == "To" {
				tLit, ok := kv.Value.(*ast.CompositeLit)
				if !ok {
					return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(kv.Value.Pos()).Line, ErrOnlyLiterals)
				}
				for _, telt := range tLit.Elts {
					sel, ok := telt.(*ast.SelectorExpr)
					name := ""
					if ok {
						name = sel.Sel.Name
					} else if id, ok := telt.(*ast.Ident); ok {
						name = id.Name
					} else {
						return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(telt.Pos()).Line, ErrOnlyLiterals)
					}

					if name == "ToRuntime" {
						s.To = append(s.To, ToRuntime)
					} else if name == "ToCI" {
						s.To = append(s.To, ToCI)
					} else if name == "ToLocal" {
						s.To = append(s.To, ToLocal)
					} else {
						return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(telt.Pos()).Line, ErrOnlyLiterals)
					}
				}
			} else {
				return nil, fmt.Errorf("%s:%d: %w", path, fset.Position(ident.Pos()).Line, ErrUnknownField)
			}
		}
		secrets = append(secrets, s)
	}
	return secrets, nil
}

func extractString(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if ok && lit.Kind == token.STRING {
		// strip quotes
		if len(lit.Value) >= 2 {
			if lit.Value[0] == '"' || lit.Value[0] == '`' {
				return lit.Value[1 : len(lit.Value)-1]
			}
		}
		return lit.Value
	}
	return ""
}

func extractText(expr ast.Expr) string {
	// since lang.Text is a string type, it's just a string or typed string literal
	if call, ok := expr.(*ast.CallExpr); ok {
		if len(call.Args) == 1 {
			return extractString(call.Args[0])
		}
	}
	return extractString(expr)
}

func extractSelector(fset *token.FileSet, path string, imports []*ast.ImportSpec, expr ast.Expr) (string, error) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", fmt.Errorf("%s:%d: %w", path, fset.Position(expr.Pos()).Line, ErrOnlyLiterals)
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf("%s:%d: %w", path, fset.Position(sel.X.Pos()).Line, ErrOnlyLiterals)
	}
	pkgName := pkgIdent.Name
	constName := sel.Sel.Name

	importPath := ""
	for _, imp := range imports {
		v := extractString(imp.Path)
		if imp.Name != nil {
			if imp.Name.Name == pkgName {
				importPath = v
				break
			}
		} else {
			parts := strings.Split(v, "/")
			if len(parts) > 0 && parts[len(parts)-1] == pkgName {
				importPath = v
				break
			}
		}
	}
	if importPath == "" {
		importPath = pkgName
	}
	return importPath + "." + constName, nil
}
