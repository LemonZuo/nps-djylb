// Package basepath normalises the web_base_url setting.
//
// NPS can be mounted under a sub-path (for example behind an nginx location
// block), configured as `web_base_url=/nps`. Every route, every asset URL and
// the SPA's own router all have to agree on that prefix, so the normalisation
// lives in one place rather than being re-derived at each call site.
package basepath

import "strings"

// Normalize cleans a configured base URL into a canonical form: either "" (the
// server is mounted at the root) or a path that starts with "/" and does not
// end with one.
//
//	""          -> ""
//	"/"         -> ""
//	"nps"       -> "/nps"
//	"/nps/"     -> "/nps"
//	"//nps//"   -> "/nps"
func Normalize(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return ""
	}
	// Collapse any interior empty segments left by inputs like "a//b".
	parts := strings.Split(raw, "/")
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return "/" + strings.Join(kept, "/")
}

// Join appends a route path to a normalised base. The route is expected to
// start with "/". Join("/nps", "/api/v1") == "/nps/api/v1".
func Join(base, route string) string {
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	if base == "" {
		return route
	}
	if route == "/" {
		// Keep the trailing slash so that the SPA root stays a directory URL.
		return base + "/"
	}
	return base + route
}

// Strip removes the base prefix from an incoming request path, reporting
// whether the path actually belonged to the base. The returned path always
// starts with "/".
//
// With base "/nps": "/nps/api/v1" -> "/api/v1", true; "/nps" -> "/", true;
// "/npsx" -> "", false.
func Strip(base, path string) (string, bool) {
	if base == "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return path, true
	}
	if path == base {
		return "/", true
	}
	if rest, ok := strings.CutPrefix(path, base+"/"); ok {
		return "/" + rest, true
	}
	return "", false
}
