package daemon

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// openBrowser opens the given URL in the default browser.
// On macOS it tries to reuse an existing tab matching the URL prefix.
func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		openBrowserMacOS(url)
	case "linux":
		_ = exec.Command("xdg-open", url).Start()
	}
}

// openBrowserMacOS tries to reuse an existing browser tab matching the URL prefix,
// falling back to opening a new tab if none is found.
func openBrowserMacOS(url string) {
	script := fmt.Sprintf(`
		-- Try Google Chrome
		if application "Google Chrome" is running then
			tell application "Google Chrome"
				set targetURL to %q
				repeat with w in windows
					set tabIndex to 0
					repeat with t in tabs of w
						set tabIndex to tabIndex + 1
						if URL of t starts with targetURL then
							set active tab index of w to tabIndex
							set index of w to 1
							tell t to reload
							activate
							return "found"
						end if
					end repeat
				end repeat
			end tell
		end if

		-- Try Safari
		if application "Safari" is running then
			tell application "Safari"
				set targetURL to %q
				repeat with w in windows
					set tabIndex to 0
					repeat with t in tabs of w
						set tabIndex to tabIndex + 1
						if URL of t starts with targetURL then
							set current tab of w to t
							set index of w to 1
							tell t to do JavaScript "location.reload()"
							activate
							return "found"
						end if
					end repeat
				end repeat
			end tell
		end if

		return "notfound"
	`, url, url)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil || strings.TrimSpace(string(out)) != "found" {
		_ = exec.Command("open", url).Start()
	}
}
