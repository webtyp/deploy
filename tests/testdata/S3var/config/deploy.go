//go:build deploy

package config

import "webtyp.com/deploy"

const name = "bad"

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name: name,
	}
}
