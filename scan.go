package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"webtyp.com/lang"
)

// DefaultFile is where a project declares its deployment.
const DefaultFile = "config/deploy.go"

const (
	buildTag   = "//go:build deploy"
	deployPath = "webtyp.com/deploy"
	deployFunc = "Deploy"
)

// Scan reads rootDir/DefaultFile and returns the Plan its Deploy function
// returns. Only literals are allowed; anything else is an error naming the
// file and line. A missing file is ErrNoPlan.
func Scan(rootDir string) (Plan, error) {
	path := filepath.Join(rootDir, DefaultFile)
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Plan{}, ErrNoPlan
	}
	if err != nil {
		return Plan{}, err
	}
	first, _, _ := bytes.Cut(src, []byte("\n"))
	if strings.TrimRight(string(first), "\r") != buildTag {
		return Plan{}, fmt.Errorf("%s:1: %w", path, ErrMissingBuildTag)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return Plan{}, err
	}
	s := scanner{fset: fset, path: path, imports: importNames(f)}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != deployFunc {
			continue
		}
		plan, err := s.deploy(fn)
		if err != nil {
			return Plan{}, err
		}
		if err := validate(plan); err != nil {
			return Plan{}, fmt.Errorf("%s: %w", path, err)
		}
		return plan, nil
	}
	return Plan{}, fmt.Errorf("%s: %w", path, ErrNoPlan)
}

// importNames maps each name the file uses for an import to its path.
func importNames(f *ast.File) map[string]string {
	names := make(map[string]string, len(f.Imports))
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = path
	}
	return names
}

// scanner reads the literal of one declaration file.
type scanner struct {
	fset    *token.FileSet
	path    string
	imports map[string]string
}

// fail is "<path>:<line>: <reason>".
func (s scanner) fail(n ast.Node, reason error) error {
	return fmt.Errorf("%s:%d: %w", s.path, s.fset.Position(n.Pos()).Line, reason)
}

func (s scanner) deploy(fn *ast.FuncDecl) (Plan, error) {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return Plan{}, s.fail(fn, ErrOnlyLiterals)
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return Plan{}, s.fail(fn.Body.List[0], ErrOnlyLiterals)
	}
	var p Plan
	err := s.fields(ret.Results[0], func(name string, v ast.Expr) error {
		var err error
		switch name {
		case "Name":
			p.Name, err = s.str(v)
		case "Domain":
			p.Domain, err = s.str(v)
		case "Target":
			var id string
			id, err = s.ref(v)
			p.Target = TargetID(id)
		case "CI":
			var id string
			id, err = s.ref(v)
			p.CI = CIID(id)
		case "Databases":
			err = s.list(v, func(e ast.Expr) error {
				var db Database
				err := s.binding(e, &db.Binding, &db.Name)
				p.Databases = append(p.Databases, db)
				return err
			})
		case "Buckets":
			err = s.list(v, func(e ast.Expr) error {
				var b Bucket
				err := s.binding(e, &b.Binding, &b.Name)
				p.Buckets = append(p.Buckets, b)
				return err
			})
		case "Vars":
			err = s.list(v, func(e ast.Expr) error {
				var vr Var
				err := s.fields(e, func(name string, v ast.Expr) error {
					var err error
					switch name {
					case "Key":
						vr.Key, err = s.str(v)
					case "Value":
						vr.Value, err = s.str(v)
					default:
						return ErrUnknownField
					}
					return err
				})
				p.Vars = append(p.Vars, vr)
				return err
			})
		case "Secrets":
			err = s.list(v, func(e ast.Expr) error {
				sec, err := s.secret(e)
				p.Secrets = append(p.Secrets, sec)
				return err
			})
		default:
			return ErrUnknownField
		}
		return err
	})
	return p, err
}

// binding reads a Database or Bucket literal.
func (s scanner) binding(e ast.Expr, binding, name *string) error {
	return s.fields(e, func(field string, v ast.Expr) error {
		var err error
		switch field {
		case "Binding":
			*binding, err = s.str(v)
		case "Name":
			*name, err = s.str(v)
		default:
			return ErrUnknownField
		}
		return err
	})
}

func (s scanner) secret(e ast.Expr) (Secret, error) {
	var sec Secret
	err := s.fields(e, func(name string, v ast.Expr) error {
		switch name {
		case "Key":
			var err error
			sec.Key, err = s.str(v)
			return err
		case "What":
			what, err := s.str(v)
			sec.What = lang.Text(what)
			return err
		case "Source":
			c, err := s.deployConst(v)
			switch c {
			case "Typed":
				sec.Source = Typed
			case "Random":
				sec.Source = Random
			default:
				if err == nil {
					err = s.fail(v, ErrInvalidSource)
				}
			}
			return err
		case "Guide":
			return s.list(v, func(e ast.Expr) error {
				step, err := s.str(e)
				sec.Guide = append(sec.Guide, lang.Text(step))
				return err
			})
		case "To":
			return s.list(v, func(e ast.Expr) error {
				c, err := s.deployConst(e)
				switch c {
				case "ToRuntime":
					sec.To = append(sec.To, ToRuntime)
				case "ToCI":
					sec.To = append(sec.To, ToCI)
				case "ToLocal":
					sec.To = append(sec.To, ToLocal)
				default:
					if err == nil {
						err = s.fail(e, ErrInvalidPlace)
					}
				}
				return err
			})
		default:
			return ErrUnknownField
		}
	})
	return sec, err
}

// fields walks a keyed composite literal. A field error that is a bare
// reason (not yet positioned) is positioned at the key.
func (s scanner) fields(e ast.Expr, each func(name string, v ast.Expr) error) error {
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return s.fail(e, ErrOnlyLiterals)
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return s.fail(elt, ErrOnlyLiterals)
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return s.fail(kv.Key, ErrOnlyLiterals)
		}
		if err := each(key.Name, kv.Value); err != nil {
			if err == ErrUnknownField {
				return s.fail(key, fmt.Errorf("%s: %w", key.Name, err))
			}
			return err
		}
	}
	return nil
}

// list walks the elements of a slice composite literal.
func (s scanner) list(e ast.Expr, each func(ast.Expr) error) error {
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return s.fail(e, ErrOnlyLiterals)
	}
	for _, elt := range lit.Elts {
		if err := each(elt); err != nil {
			return err
		}
	}
	return nil
}

// str reads a string literal ("..." or raw).
func (s scanner) str(e ast.Expr) (string, error) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", s.fail(e, ErrOnlyLiterals)
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", s.fail(e, ErrOnlyLiterals)
	}
	return v, nil
}

// selector splits pkg.Name, resolving pkg through the file's imports.
func (s scanner) selector(e ast.Expr) (path, name string, err error) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", "", s.fail(e, ErrOnlyLiterals)
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", s.fail(e, ErrOnlyLiterals)
	}
	path, ok = s.imports[pkg.Name]
	if !ok {
		return "", "", s.fail(e, fmt.Errorf("%s: %w", pkg.Name, ErrUnknownImport))
	}
	return path, sel.Sel.Name, nil
}

// ref reads a Target or CI constant as "<import path>.<constant>".
func (s scanner) ref(e ast.Expr) (string, error) {
	path, name, err := s.selector(e)
	if err != nil {
		return "", err
	}
	return path + "." + name, nil
}

// deployConst reads a selector on the webtyp.com/deploy import.
func (s scanner) deployConst(e ast.Expr) (string, error) {
	path, name, err := s.selector(e)
	if err != nil {
		return "", err
	}
	if path != deployPath {
		return "", s.fail(e, fmt.Errorf("%s: %w", path, ErrUnknownImport))
	}
	return name, nil
}
