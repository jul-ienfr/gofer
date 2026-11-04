package i18n

import "testing"

func TestNormalizeAndResolve(t *testing.T) {
	for _, test := range []struct {
		raw    string
		locale Locale
		ok     bool
	}{
		{"en", English, true},
		{"EN-us", English, true},
		{"fr", French, true},
		{"fr-FR", French, true},
		{"fr_CA", French, true},
		{"de", English, false},
		{"", English, false},
	} {
		locale, ok := Normalize(test.raw)
		if locale != test.locale || ok != test.ok {
			t.Fatalf("Normalize(%q) = %q, %v; want %q, %v", test.raw, locale, ok, test.locale, test.ok)
		}
		if resolved := Resolve(test.raw); resolved != test.locale {
			t.Fatalf("Resolve(%q) = %q; want %q", test.raw, resolved, test.locale)
		}
	}
}

func TestCatalogParity(t *testing.T) {
	for _, locale := range Supported() {
		catalog, ok := messages[locale]
		if !ok || len(catalog) == 0 {
			t.Fatalf("catalog %q is missing or empty", locale)
		}
		for key := range messages[English] {
			if catalog[key] == "" {
				t.Fatalf("locale %q is missing key %q", locale, key)
			}
		}
		for key := range catalog {
			if messages[English][key] == "" {
				t.Fatalf("locale %q has an unknown key %q", locale, key)
			}
		}
	}
}

func TestFallbackAndInterpolation(t *testing.T) {
	if got := T(French, "settings.title"); got != "Paramètres" {
		t.Fatalf("French title = %q", got)
	}
	if got := T(French, "missing.key"); got != "missing.key" {
		t.Fatalf("missing key = %q", got)
	}
	messages[French]["test.greeting"] = "Bonjour %s"
	defer delete(messages[French], "test.greeting")
	if got := Tf(French, "test.greeting", "Alice"); got != "Bonjour Alice" {
		t.Fatalf("formatted greeting = %q", got)
	}
	if got := NativeName(French); got != "Français" {
		t.Fatalf("native name = %q", got)
	}
	if got := NativeName(Locale("de")); got != "English" {
		t.Fatalf("unknown native name = %q", got)
	}
}

func TestPlural(t *testing.T) {
	for _, test := range []struct {
		locale Locale
		count  int
		want   string
	}{
		{English, 0, "other"},
		{English, 1, "one"},
		{English, 2, "other"},
		{French, 0, "one"},
		{French, 1, "one"},
		{French, 2, "other"},
	} {
		if got := Plural(test.locale, test.count); got != test.want {
			t.Fatalf("Plural(%q, %d) = %q; want %q", test.locale, test.count, got, test.want)
		}
	}
}

func TestTPUsesPluralKey(t *testing.T) {
	messages[English]["test.one"] = "%d message"
	messages[English]["test.other"] = "%d messages"
	defer delete(messages[English], "test.one")
	defer delete(messages[English], "test.other")
	if got := TP(English, 1, "test.one", "test.other", 1); got != "1 message" {
		t.Fatalf("one = %q", got)
	}
	if got := TP(English, 2, "test.one", "test.other", 2); got != "2 messages" {
		t.Fatalf("other = %q", got)
	}
}
