// Package service installs and manages the Yucca daemon as an OS-managed
// background service, so it starts at login, restarts on crash, and lives
// independently of any agent (Codex/Claude) MCP session or the tray app.
//
// Each supported OS provides the same small API via build-tagged files:
//
//	WriteDescriptor(binPath) (changed bool, err error)  persist the service unit
//	Start(reload bool) error                             load + (re)start it now
//	Stop() error                                         stop + unload it
//	Remove() error                                       stop + delete the unit
//	Available() bool                                     is a service manager usable here
//
//	macOS  launchd LaunchAgent (~/Library/LaunchAgents/co.kobylinski.yucca.daemon.plist)
//	Linux  systemd --user unit (~/.config/systemd/user/yucca-daemon.service)
//
// The daemon is always run with --idle-timeout 0 so the manager keeps it up.
package service
