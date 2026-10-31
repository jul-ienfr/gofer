package handler

import (
	"net/http"
	"path"
	"strings"
)

// minifiedScripts serves the generated min/js copy of a script under the
// script's public /js/name.min.js path, and every other file as it is.
type minifiedScripts struct{ base http.FileSystem }

func (m minifiedScripts) Open(name string) (http.File, error) {
	if minified, ok := minifiedScriptName(name); ok {
		if file, err := m.base.Open(minified); err == nil {
			return file, nil
		}
	}
	return m.base.Open(name)
}

// minifiedScriptName maps a public /js/name.min.js path to the generated
// /min/js/name.js asset. Other paths are left to the underlying filesystem.
func minifiedScriptName(name string) (string, bool) {
	name = path.Clean("/" + name)
	if !strings.HasPrefix(name, "/js/") || !strings.HasSuffix(name, ".min.js") {
		return "", false
	}
	base := strings.TrimSuffix(strings.TrimPrefix(name, "/js/"), ".min.js")
	if base == "" || strings.Contains(base, "/") {
		return "", false
	}
	return "/min/js/" + base + ".js", true
}
