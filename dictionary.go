package deploy

import _ "embed"

//go:embed lang.json
var dictionary []byte

func Dictionary() []byte {
	return dictionary
}
