package reviewprompt

import _ "embed"

//go:embed templates/review-agent.md
var defaultTemplate string

//go:embed templates/reviewer-roles.json
var roleOverlaysJSON string
