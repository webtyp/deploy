//go:build deploy

package config

import (
	d "webtyp.com/deploy"
	alias "webtyp.com/github"
	"webtyp.com/goflare"
	"webtyp.com/lang"
)

func Deploy() d.Plan {
	return d.Plan{
		Name:      "misitio",
		Domain:    `misitio.velty.cl`,
		Target:    goflare.Worker,
		CI:        alias.Actions,
		Databases: []d.Database{{Binding: "DB", Name: "velty-misitio-db"}},
		Buckets:   []d.Bucket{d.Bucket{Binding: "ASSETS", Name: "velty-misitio-assets"}},
		Vars:      []d.Var{{Key: "IAM_BASE_URL", Value: "https://iam.velty.cl"}},
		Secrets: []d.Secret{
			{
				Key:    "STRIPE_KEY",
				What:   "Stripe API key",
				Source: d.Typed,
				Guide:  []lang.Text{"Open the Stripe dashboard", `Copy the "secret key"`},
				To:     []d.Place{d.ToRuntime, d.ToLocal},
			},
			{
				Key:    "SESSION_KEY",
				What:   "Session signing key",
				Source: d.Random,
				To:     []d.Place{d.ToRuntime},
			},
		},
	}
}
