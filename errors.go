package deploy

import "errors"

var (
	ErrNoPlan             = errors.New("no deployment declared: create config/deploy.go with func Deploy() deploy.Plan")
	ErrMissingBuildTag    = errors.New("config/deploy.go must start with //go:build deploy")
	ErrNotReady           = errors.New("deployment not ready")
	ErrOnlyLiterals       = errors.New("only literals are allowed")
	ErrUnknownField       = errors.New("unknown field")
	ErrUnknownImport      = errors.New("selector does not name an import of this file")
	ErrNoName             = errors.New("Plan.Name is empty")
	ErrNoTarget           = errors.New("Plan.Target is empty")
	ErrNoCI               = errors.New("Plan.CI is empty")
	ErrTargetMismatch     = errors.New("target mismatch")
	ErrCIMismatch         = errors.New("CI mismatch")
	ErrDuplicateSecret    = errors.New("duplicate or empty secret key")
	ErrInvalidSource      = errors.New("invalid secret source")
	ErrInvalidPlace       = errors.New("invalid or duplicate secret place")
	ErrNoPlace            = errors.New("secret has no places")
	ErrNoGuide            = errors.New("typed secret missing guide")
	ErrNoWhat             = errors.New("secret missing what")
	ErrIncompleteDatabase = errors.New("database missing binding or name")
	ErrIncompleteBucket   = errors.New("bucket missing binding or name")
	ErrIncompleteVar      = errors.New("var missing key or value")
	ErrDuplicateVar       = errors.New("declared twice, as a Var or Secret")
	ErrNilTool            = errors.New("tool is nil")
)
