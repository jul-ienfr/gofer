package a11y

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

type Violation struct {
	Rule    string
	Message string
}

type Report struct {
	Violations []Violation
}

func Audit(r io.Reader) (Report, error) {
	root, err := html.Parse(r)
	if err != nil {
		return Report{}, err
	}
	ids := map[string]*html.Node{}
	idCounts := map[string]int{}
	labelsByFor := map[string][]string{}
	var report Report
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if id := attr(node, "id"); id != "" {
				if !isHidden(node) {
					idCounts[id]++
				}
				ids[id] = node
			}
			if node.Data == "label" {
				if target := attr(node, "for"); target != "" {
					labelsByFor[target] = append(labelsByFor[target], text(node))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	hasMain := false
	hasSkipLink := false
	hasNavigation := false
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if isHidden(node) {
				return
			}
			switch node.Data {
			case "html":
				if strings.TrimSpace(attr(node, "lang")) == "" {
					add(&report, "html-lang", "the document does not declare a language")
				}
			case "title":
				if strings.TrimSpace(text(node)) == "" {
					add(&report, "document-title", "the document has an empty title")
				}
			case "main":
				hasMain = true
			case "nav", "aside":
				hasNavigation = true
			case "img":
				if _, ok := attrOK(node, "alt"); !ok {
					add(&report, "image-alt", "an image has no alt attribute")
				}
			case "input":
				checkControl(node, ids, labelsByFor, &report)
			case "select", "textarea":
				if accessibleName(node, ids, labelsByFor) == "" {
					add(&report, "control-name", "a form control has no accessible name: "+describe(node))
				}
			case "button":
				if accessibleName(node, ids, labelsByFor) == "" {
					add(&report, "button-name", "a button has no accessible name: "+describe(node))
				}
			case "a":
				if href, ok := attrOK(node, "href"); ok && href != "" {
					if accessibleName(node, ids, labelsByFor) == "" {
						add(&report, "link-name", "a link has no accessible name")
					}
					if href == "#main-content" {
						hasSkipLink = true
					}
				}
			}
			switch attr(node, "role") {
			case "main":
				hasMain = true
			case "navigation":
				hasNavigation = true
			}
			if tabindex, ok := attrOK(node, "tabindex"); ok {
				if positiveTabIndex(tabindex) {
					add(&report, "tabindex", "a positive tabindex overrides the natural focus order")
				}
			}
			if labelledBy := strings.TrimSpace(attr(node, "aria-labelledby")); labelledBy != "" {
				for _, id := range strings.Fields(labelledBy) {
					if ids[id] == nil {
						add(&report, "aria-labelledby", "aria-labelledby references a missing id")
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)

	for id, count := range idCounts {
		if count > 1 {
			add(&report, "duplicate-id", "id "+id+" is used more than once")
		}
	}
	if !hasMain {
		add(&report, "main-landmark", "the document has no main landmark")
	}
	if hasNavigation && !hasSkipLink {
		add(&report, "skip-link", "the document has no skip link to #main-content")
	}
	return report, nil
}

func checkControl(node *html.Node, ids map[string]*html.Node, labels map[string][]string, report *Report) {
	switch strings.ToLower(attr(node, "type")) {
	case "hidden":
		return
	case "submit", "button", "reset":
		if strings.TrimSpace(attr(node, "value")) != "" || accessibleName(node, ids, labels) != "" {
			return
		}
		add(report, "control-name", "a button input has no accessible name")
		return
	case "image":
		if strings.TrimSpace(attr(node, "alt")) != "" || accessibleName(node, ids, labels) != "" {
			return
		}
		add(report, "control-name", "an image input has no accessible name")
		return
	}
	if accessibleName(node, ids, labels) == "" {
		add(report, "control-name", "a form control has no accessible name: "+describe(node))
	}
}

func accessibleName(node *html.Node, ids map[string]*html.Node, labels map[string][]string) string {
	if label := strings.TrimSpace(attr(node, "aria-label")); label != "" {
		return label
	}
	if labelledBy := strings.TrimSpace(attr(node, "aria-labelledby")); labelledBy != "" {
		var parts []string
		for _, id := range strings.Fields(labelledBy) {
			if target := ids[id]; target != nil {
				if label := strings.TrimSpace(text(target)); label != "" {
					parts = append(parts, label)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	if title := strings.TrimSpace(attr(node, "title")); title != "" {
		return title
	}
	if node.Data == "input" {
		if alt := strings.TrimSpace(attr(node, "alt")); alt != "" {
			return alt
		}
	}
	if id := attr(node, "id"); id != "" {
		for _, label := range labels[id] {
			if strings.TrimSpace(label) != "" {
				return label
			}
		}
	}
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && parent.Data == "label" {
			if label := strings.TrimSpace(text(parent)); label != "" {
				return label
			}
		}
	}
	if node.Data == "button" || node.Data == "a" {
		if label := strings.TrimSpace(text(node)); label != "" {
			return label
		}
	}
	return ""
}

func text(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return b.String()
}

func attr(node *html.Node, name string) string {
	value, _ := attrOK(node, name)
	return value
}

func attrOK(node *html.Node, name string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val, true
		}
	}
	return "", false
}

func describe(node *html.Node) string {
	description := "<" + node.Data
	if id := attr(node, "id"); id != "" {
		description += " id=" + id
	}
	if name := attr(node, "name"); name != "" {
		description += " name=" + name
	}
	if kind := attr(node, "type"); kind != "" {
		description += " type=" + kind
	}
	if class := attr(node, "class"); class != "" {
		if len(class) > 80 {
			class = class[:80]
		}
		description += " class=" + class
	}
	for _, attribute := range node.Attr {
		if strings.HasPrefix(attribute.Key, "data-") && attribute.Val != "" && len(attribute.Val) < 60 {
			description += " " + attribute.Key + "=" + attribute.Val
		}
	}
	return description + ">"
}

func add(report *Report, rule, message string) {
	for _, existing := range report.Violations {
		if existing.Rule == rule && existing.Message == message {
			return
		}
	}
	report.Violations = append(report.Violations, Violation{Rule: rule, Message: message})
}

func isHidden(node *html.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type != html.ElementNode {
			continue
		}
		if current.Data == "template" {
			return true
		}
		if current.Data == "dialog" {
			if _, open := attrOK(current, "open"); !open {
				return true
			}
		}
		if _, ok := attrOK(current, "hidden"); ok {
			return true
		}
		if attr(current, "aria-hidden") == "true" {
			return true
		}
	}
	return false
}

func positiveTabIndex(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != "" && value != "0" && strings.TrimLeft(value, "0") != ""
}
