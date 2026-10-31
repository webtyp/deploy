# deploy
<img src="docs/img/badges.svg">

`webtyp.com/deploy` is the deployment contract shared by all webtyp applications and providers. It declares what a project needs and orchestrates how a CI and a target work together to fulfill those needs. It contains only types, interfaces, the reader and writer of a project's declaration, and the generic orchestration over those interfaces.

It does not contain any provider implementations (like Cloudflare or GitHub). Implementations import `deploy`, and `webtyp.com/app` composes them.

## Why This Exists

This package centralizes the definition of a deployment (`Plan`) and the lifecycle to enforce it (`Deployment`).
It provides `deploy.Scan` to read a project's `config/deploy.go` into a `Plan`, and `deploy.Write` to serialize a `Plan` back into `config/deploy.go`. This enables both manual editing by users and automated scaffolding by tools like `webtyp/app`.

## Before You Start

- You must import `webtyp.com/deploy` and provider implementations in your `config/deploy.go`.
- Tool implementations must import this package, but this package never imports them.

## Step-by-Step Guide

Here is a complete example of generating and scanning a deployment plan:

```go
package main

import (
	"fmt"
	"log"

	"webtyp.com/deploy"
)

func main() {
	plan := deploy.Plan{
		Name:   "example-app",
		Target: "webtyp.com/goflare.Worker",
		CI:     "webtyp.com/github.Actions",
	}

	// 1. Write the plan to config/deploy.go
	if err := deploy.Write(".", plan); err != nil {
		log.Fatalf("failed to write plan: %v", err)
	}

	// 2. Scan the written file back into a Plan
	scanned, err := deploy.Scan(".")
	if err != nil {
		log.Fatalf("failed to scan plan: %v", err)
	}

	fmt.Printf("Scanned Plan: %+v\n", scanned)
}
```

## Documentation Map

- **`Plan`**: The deployment specification.
- **`Deployment`**: A `Plan` combined with the tools (`Target`, `CI`, `Local` destination) that carry it out.
- **`Target`**: Where the project runs.
- **`CI`**: The platform that runs the deployment.
- **`Destination`**: An interface representing places that receive secrets.
- **`Missing`**: Items (secrets or configuration) that are required but absent.
