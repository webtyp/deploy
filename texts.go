package deploy

import "webtyp.com/lang"

const (
	textWhyRuntime lang.Text = "The deployed app cannot start without it"
	textWhyCI      lang.Text = "The CI cannot deploy without it"
	textWhyLocal   lang.Text = "The local development server needs it"
)
