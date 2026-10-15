//go:build deploy

package config

import (
	"strings"

	"webtyp.com/deploy"
)

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name: strings.ToLower("BAD"),
	}
}
