//go:build deploy

package config

import (
	"webtyp.com/deploy"
	alias "webtyp.com/github"
	"webtyp.com/goflare"
)

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:      "misitio",
		Domain:    "misitio.velty.cl",
		Target:    goflare.Worker,
		CI:        alias.Actions,
		Databases: []deploy.Database{{Binding: "DB", Name: "velty-misitio-db"}},
		Buckets:   []deploy.Bucket{{Binding: "ASSETS", Name: "velty-misitio-assets"}},
		Vars:      []deploy.Var{{Key: "IAM_BASE_URL", Value: "https://iam.velty.cl"}},
		Secrets: []deploy.Secret{
			{
				Key:    "STRIPE_KEY",
				What:   "Stripe API Key",
				Source: deploy.Typed,
				Guide:  []deploy.Text{"Go to Stripe", "Copy key"},
				To:     []deploy.Place{deploy.ToRuntime, deploy.ToLocal},
			},
		},
	}
}
