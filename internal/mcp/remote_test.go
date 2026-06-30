package mcp

import "testing"

func TestParseRemotePath(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		path   string
		remote bool
	}{
		{"deploy@server:~/app/.env", "deploy@server", "~/app/.env", true},
		{"server:/etc/app/.env", "server", "/etc/app/.env", true},
		{"/Users/marek/proj/.env", "", "", false},
		{"./relative/.env", "", "", false},
		{".env", "", "", false},
		{"host:", "", "", false},
		{":path", "", "", false},
		{"/abs:withcolon/.env", "", "", false}, // colon after a slash → local
	}
	for _, c := range cases {
		h, p, r := parseRemotePath(c.in)
		if r != c.remote || h != c.host || p != c.path {
			t.Errorf("parseRemotePath(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, h, p, r, c.host, c.path, c.remote)
		}
	}
}

func TestRemoteShellPath(t *testing.T) {
	cases := map[string]string{
		"~/app/.env":    "~/'app/.env'",
		"~":             "~",
		"/etc/app/.env": "'/etc/app/.env'",
		"a b/c.env":     "'a b/c.env'",
		"it's/x":        `'it'\''s/x'`,
	}
	for in, want := range cases {
		if got := remoteShellPath(in); got != want {
			t.Errorf("remoteShellPath(%q) = %q, want %q", in, got, want)
		}
	}
}
