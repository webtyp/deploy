package deploy

// Destination receives secrets. It is write-only: it can say whether a key
// exists, never return a value.
type Destination interface {
	Has(key string) (bool, error)
	Put(key, value string) error
}

// Input is one named input of a CI job.
type Input struct{ Name, Value string }

// Job is the single CI step a Target needs to deploy: an action reference,
// its inputs, and the names of the CI secrets it reads as environment variables.
type Job struct {
	Uses    string // e.g. "tinywasm/goflare@v1"
	With    []Input
	Secrets []string // e.g. "CLOUDFLARE_API_TOKEN"
}

// Target is where the project runs (implemented by webtyp/goflare, ...).
type Target interface {
	ID() TargetID
	// Check reports what the target still needs (credentials, resources,
	// its own CI secrets in ci). It changes nothing.
	Check(p Plan, ci Destination) ([]Missing, error)
	// Provision creates what is missing and delivers the target's own CI
	// secrets to ci. Idempotent: never replaces a secret that exists.
	Provision(p Plan, ci Destination) error
	// Rotate replaces every production secret the target owns.
	Rotate(p Plan, ci Destination) error
	// Runtime is where ToRuntime secrets go.
	Runtime(p Plan) Destination
	// Job is the CI step that deploys p.
	Job(p Plan) Job
}

// RunState is the state of a CI run.
type RunState uint8

const (
	RunQueued RunState = iota + 1
	RunInProgress
	RunSucceeded
	RunFailed
)

// Run identifies one CI run.
type Run struct {
	ID  string
	URL string
}

// CI runs the deploy (implemented by webtyp/github, ...).
type CI interface {
	ID() CIID
	Check(p Plan, job Job) ([]Missing, error)
	Provision(p Plan, job Job) error // writes or updates the deploy workflow
	Secrets(p Plan) Destination      // where ToCI secrets go
	Release(p Plan) (Run, error)
	Status(r Run) (RunState, error)
}
