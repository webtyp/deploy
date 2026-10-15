# deploy
<img src="docs/img/badges.svg">

`webtyp.com/deploy` is the deployment contract shared by all webtyp applications and providers. It declares what a project needs and orchestrates how a CI and a target work together to fulfill those needs. It contains only types, interfaces, the reader of a project's declaration, and the generic orchestration over those interfaces.

It does not contain any provider implementations (like Cloudflare or GitHub). Implementations import `deploy`, and `webtyp.com/app` composes them.

## Types and Contracts

- **`Plan`**: The deployment specification.
- **`Deployment`**: A `Plan` combined with the tools (`Target`, `CI`, `Local` destination) that carry it out.
- **`Target`**: Where the project runs (e.g., a Cloudflare Worker).
- **`CI`**: The platform that runs the deployment (e.g., GitHub Actions).
- **`Destination`**: An interface representing places that receive secrets (Runtime, CI, Local keyring).
- **`Missing`**: Items (secrets or configuration) that are required but absent.

## Example Configuration

Projects declare their deployment by placing a `config/deploy.go` file with the following exact format:

```go
//go:build deploy

package config

import (
	"webtyp.com/deploy"
	"webtyp.com/github"
	"webtyp.com/goflare"
)

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:      "misitio",
		Domain:    "misitio.velty.cl",
		Target:    goflare.Worker,
		CI:        github.Actions,
		Databases: []deploy.Database{{Binding: "DB", Name: "velty-misitio-db"}},
		Buckets:   []deploy.Bucket{{Binding: "ASSETS", Name: "velty-misitio-assets"}},
		Vars:      []deploy.Var{{Key: "IAM_BASE_URL", Value: "https://iam.velty.cl"}},
	}
}
```

## Orchestration Methods

The `Deployment` struct exposes the following orchestration methods:

- **`Check`**: Validates the plan and queries the `CI`, `Target`, and all specified places (Runtime, CI, Local) for missing secrets. Returns a list of `Missing` items.
- **`LocalMissing`**: A scoped check that verifies what secrets are missing from the local development server (Local Keyring).
- **`Provision`**: Automatically creates any missing `Random` secrets and delivers them to their required destinations. Asks `Target` and `CI` to provision themselves.
- **`Rotate`**: Regenerates and overwrites all `Random` secrets across all destinations, and returns a list of `FixInput` missing items for any `Typed` secrets.
- **`Release`**: Verifies the deployment is ready via `Check`, and triggers the CI deploy job.
- **`Status`**: Returns the current status of the CI deployment run.

**Important**: Tool implementations MUST import `deploy`; this package will never import an implementation.