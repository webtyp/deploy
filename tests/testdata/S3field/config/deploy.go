//go:build deploy

package config

import "webtyp.com/deploy"

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:    "bad",
		Unknown: "field",
	}
}
