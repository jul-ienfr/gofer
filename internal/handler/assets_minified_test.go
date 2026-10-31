package handler

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMinifiedScriptsPrefersGeneratedCopy(t *testing.T) {
	fs := minifiedScripts{http.FS(fstest.MapFS{
		"js/input.min.js": {Data: []byte("stale")},
		"min/js/input.js": {Data: []byte("generated")},
		"js/htmx.min.js":  {Data: []byte("htmx")},
		"js/app.min.js":   {Data: []byte("app")},
		"min/js/app.js":   {Data: []byte("generated app")},
		"js/source.js":    {Data: []byte("source")},
	})}

	for name, want := range map[string]string{
		"js/input.min.js": "generated",
		"js/htmx.min.js":  "htmx",
		"js/app.min.js":   "generated app",
		"js/source.js":    "source",
	} {
		file, err := fs.Open(name)
		if err != nil {
			t.Fatalf("Open(%q) error = %v", name, err)
		}
		got, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatalf("ReadAll(%q) error = %v", name, err)
		}
		if strings.TrimSpace(string(got)) != want {
			t.Errorf("Open(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestMinifiedScriptNameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"/js/../secret.min.js", "/js/nested/app.min.js", "/js/.min.js", "/css/app.min.js"} {
		if got, ok := minifiedScriptName(name); ok {
			t.Errorf("minifiedScriptName(%q) = %q, true; want false", name, got)
		}
	}
}
