// Package agent carries the Aeolus agent's files, which a manager build
// offers its APs as one bundle (0079).
package agent

import "embed"

// Files is the agent's files, each at its place on the AP under "files":
// files/usr/sbin/aeolus-agent is /usr/sbin/aeolus-agent.
//
//go:embed files
var Files embed.FS
