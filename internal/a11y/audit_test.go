package a11y

import (
	"strings"
	"testing"
)

func TestAuditAcceptsAccessiblePage(t *testing.T) {
	page := `<!doctype html>
<html lang="en">
<head><title>Mail</title></head>
<body>
<a href="#main-content">Skip to content</a>
<main id="main-content">
<h1>Inbox</h1>
<img src="logo.png" alt="Gofer">
<label for="search">Search</label><input id="search" type="search">
<button type="submit">Send</button>
<a href="/settings">Settings</a>
</main>
</body>
</html>`
	report, err := Audit(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("unexpected violations: %#v", report.Violations)
	}
}

func TestAuditSkipsHiddenDialogsAndTemplates(t *testing.T) {
	page := `<!doctype html>
<html lang="en">
<head><title>Mail</title></head>
<body>
<a href="#main-content">Skip to content</a>
<nav>Folders</nav>
<main id="main-content">
<dialog><button></button><input></dialog>
<template><button></button><img src="x"></template>
<input type="text" hidden>
</main>
</body>
</html>`
	report, err := Audit(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("unexpected violations: %#v", report.Violations)
	}
}

func TestAuditFindsStructuralProblems(t *testing.T) {
	page := `<!doctype html>
<html>
<head><title></title></head>
<body>
<img src="logo.png">
<label for="search">Search</label><input id="search">
<input id="search" type="email">
<input type="text">
<button></button>
<a href="/settings"><svg></svg></a>
<a href="#main-content">Skip</a>
<div id="main-content" tabindex="2"></div>
<div aria-labelledby="missing"></div>
</body>
</html>`
	report, err := Audit(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"html-lang":       true,
		"document-title":  true,
		"image-alt":       true,
		"control-name":    true,
		"button-name":     true,
		"link-name":       true,
		"main-landmark":   true,
		"tabindex":        true,
		"aria-labelledby": true,
		"duplicate-id":    true,
	}
	for _, violation := range report.Violations {
		delete(want, violation.Rule)
	}
	if len(want) != 0 {
		t.Fatalf("missing violations %v; got %#v", want, report.Violations)
	}
}
