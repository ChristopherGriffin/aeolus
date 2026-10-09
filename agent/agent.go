// Package agent carries the Aeolus agent's files, which a manager build
// offers its APs as one bundle (0079).
package agent

import "embed"

// Files is the agent's files, each at its place on the AP under "files":
// files/usr/sbin/aeolus-agent is /usr/sbin/aeolus-agent.
//
//go:embed files
var Files embed.FS

// LuCI is LuCI's page for enrolling an AP by hand (0083), each file at its
// place on the AP under "luci". The manager's installer puts it beside the
// agent where LuCI is installed; a fleet update never carries it.
//
//go:embed luci
var LuCI embed.FS

// WebInstall is the script the manager serves at /install (0083).
//
//go:embed web-install.sh
var WebInstall []byte
