package deploy

import "webtyp.com/lang"

// Fix says how a Missing item is resolved.
type Fix uint8

const (
	FixAutomatic Fix = iota + 1 // Deployment.Provision resolves it, no input needed
	FixInput                    // a person types a value; Accept receives it
	FixManual                   // a person acts outside webtyp (Steps), then checks again
)

// Missing is one thing a deployment still needs. It never carries a secret value.
type Missing struct {
	Item   lang.Text   // short name, e.g. "Cloudflare bootstrap token"
	Why    lang.Text   // what breaks without it
	Steps  []lang.Text // how to get it (FixInput, FixManual)
	Fix    Fix
	Secret bool                     // FixInput: the value must be masked
	Accept func(value string) error // FixInput only: stores or delivers the value
}
