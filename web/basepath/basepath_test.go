package basepath

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"/":        "",
		"///":      "",
		"   ":      "",
		"nps":      "/nps",
		"/nps":     "/nps",
		"/nps/":    "/nps",
		"//nps//":  "/nps",
		" /nps/ ":  "/nps",
		"a/b":      "/a/b",
		"/a//b/":   "/a/b",
		"/admin/v": "/admin/v",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoin(t *testing.T) {
	cases := []struct {
		base, route, want string
	}{
		{"", "/api/v1", "/api/v1"},
		{"", "/", "/"},
		{"", "api/v1", "/api/v1"},
		{"/nps", "/api/v1", "/nps/api/v1"},
		{"/nps", "api/v1", "/nps/api/v1"},
		{"/nps", "/", "/nps/"},
	}
	for _, c := range cases {
		if got := Join(c.base, c.route); got != c.want {
			t.Errorf("Join(%q, %q) = %q, want %q", c.base, c.route, got, c.want)
		}
	}
}

func TestStrip(t *testing.T) {
	cases := []struct {
		base, path, want string
		wantOK           bool
	}{
		{"", "/api/v1", "/api/v1", true},
		{"", "api/v1", "/api/v1", true},
		{"/nps", "/nps/api/v1", "/api/v1", true},
		{"/nps", "/nps/", "/", true},
		{"/nps", "/nps", "/", true},
		// A path that merely shares a textual prefix must not be treated as
		// belonging to the base, or /npsadmin would leak into the SPA.
		{"/nps", "/npsx", "", false},
		{"/nps", "/npsx/api", "", false},
		{"/nps", "/other", "", false},
		{"/nps", "/", "", false},
	}
	for _, c := range cases {
		got, ok := Strip(c.base, c.path)
		if got != c.want || ok != c.wantOK {
			t.Errorf("Strip(%q, %q) = (%q, %v), want (%q, %v)", c.base, c.path, got, ok, c.want, c.wantOK)
		}
	}
}

func TestJoinStripRoundTrip(t *testing.T) {
	for _, base := range []string{"", "/nps", "/a/b"} {
		for _, route := range []string{"/api/v1/auth/login", "/assets/index.js"} {
			joined := Join(base, route)
			got, ok := Strip(base, joined)
			if !ok || got != route {
				t.Errorf("round trip base=%q route=%q: joined=%q -> (%q, %v)", base, route, joined, got, ok)
			}
		}
	}
}
