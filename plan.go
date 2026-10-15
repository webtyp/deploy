package deploy

import "webtyp.com/lang"

// TargetID names a deployment target implementation as "<import path>.<constant>",
// e.g. "webtyp.com/goflare.Worker". The implementing package exports a constant of
// this type whose value is exactly that text; a project writes the constant.
type TargetID string

// CIID names a CI implementation the same way, e.g. "webtyp.com/github.Actions".
type CIID string

// Plan is what a project declares in config/deploy.go.
type Plan struct {
	Name      string   // service name, e.g. "misitio"
	Domain    string   // custom domain; "" = none
	Target    TargetID // where it runs
	CI        CIID     // who runs the deploy
	Databases []Database
	Buckets   []Bucket
	Secrets   []Secret
	Vars      []Var
}

// Var is a non-secret runtime variable, value written in config/deploy.go,
// delivered by the Target as plain-text bindings on every deploy.
type Var struct {
	Key   string
	Value string
}

// Database is a SQL database the deployed code reaches through Binding.
type Database struct {
	Binding string // name the code uses, e.g. "DB"
	Name    string // resource name at the provider, e.g. "velty-misitio-db"
}

// Bucket is an object store the deployed code reaches through Binding.
type Bucket struct {
	Binding string
	Name    string
}

// Source says where a secret's value comes from.
type Source uint8

const (
	Typed  Source = iota + 1 // a person types it in the wizard (provider without an API)
	Random                   // generated: 32 random bytes, base64url without padding
)

// Place says who reads a secret.
type Place uint8

const (
	ToRuntime Place = iota + 1 // the deployed app, at run time
	ToCI                       // the CI job that deploys
	ToLocal                    // the local development server of each person
)

// Secret is a value the project needs that grants access.
type Secret struct {
	Key    string      // e.g. "IAM_CLIENT_PRIVATE_KEY"
	What   lang.Text   // what it is, in one plain sentence (English; translated)
	Source Source
	Guide  []lang.Text // for Typed: the steps to obtain it, shown one per line
	To     []Place
}
