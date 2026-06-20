// Package assets exposes the repo's static assets as an embedded FS so binaries
// built without the full source tree can still read them.
package assets

import "embed"

// SkillFS contains the portable Pactum agent skill package. The embedded tree
// mirrors assets/agent-skills/pactum/ and is the source of truth for
// pactum skill install.
//
//go:embed agent-skills/pactum
var SkillFS embed.FS
