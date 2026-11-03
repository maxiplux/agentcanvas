// Package builtinskills embeds the built-in agentcanvas skill (§14.3).
package builtinskills

import "embed"

// FS holds agentcanvas/SKILL.md and agentcanvas/skill.json.
//
//go:embed agentcanvas
var FS embed.FS
