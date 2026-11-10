package views

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/i18n"
	"github.com/cristianadrielbraun/gofer/internal/models"
)

func TestRegionalSettingsRenderInTheRequestLanguage(t *testing.T) {
	render := func(locale i18n.Locale, settings map[string]string) string {
		var out strings.Builder
		ctx := i18n.WithLocale(context.Background(), locale)
		if err := SettingsPage(nil, models.SyncSettings{}, "regional", settings, nil, AISettingsData{}).Render(ctx, &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	english := render(i18n.English, map[string]string{})
	if !strings.Contains(english, ">Language</h4>") || !strings.Contains(english, "Automatic") {
		t.Fatal("English page lacks the Language row")
	}
	spanish := render("es", map[string]string{i18n.SettingKey: "es"})
	for _, want := range []string{">Idioma</h4>", ">Zona horaria</h4>", "Idioma y región", `value="es"`} {
		if !strings.Contains(spanish, want) {
			t.Errorf("Spanish page lacks %q", want)
		}
	}
	if strings.Contains(spanish, ">Timezone</h4>") {
		t.Error("Spanish page still shows the English Timezone heading")
	}
	// Each language is listed in its own name, whatever the page language.
	if !strings.Contains(spanish, "English") || !strings.Contains(english, "Español") {
		t.Error("language names are not shown in their own language")
	}
}

func TestPageLangFollowsTheRequest(t *testing.T) {
	if got := pageLang(i18n.WithLocale(context.Background(), "es")); got != "es" {
		t.Fatalf("pageLang = %q", got)
	}
	if got := pageLang(context.Background()); got != "en" {
		t.Fatalf("default pageLang = %q", got)
	}
}

func TestEveryHTMLRootDeclaresDirection(t *testing.T) {
	files, err := filepath.Glob("*.templ")
	if err != nil {
		t.Fatal(err)
	}
	roots := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "<html lang={ pageLang(ctx) }") {
				continue
			}
			roots++
			if !strings.Contains(line, "dir={ pageDirection(ctx) }") {
				t.Errorf("%s: HTML root lacks dir: %s", file, strings.TrimSpace(line))
			}
		}
	}
	if roots < 20 {
		t.Fatalf("found only %d templ HTML roots", roots)
	}
	management, err := os.ReadFile("management.go")
	if err != nil {
		t.Fatal(err)
	}
	rawRoots := strings.Count(string(management), `<html lang="`)
	directedRoots := strings.Count(string(management), `pageDirection(ctx)`)
	if rawRoots != 5 || directedRoots != rawRoots {
		t.Fatalf("management HTML roots = %d, with direction = %d, want 5 each", rawRoots, directedRoots)
	}
}

// Lists of names use the page language's comma: Chinese writes "A、B 和 C".
func TestListsUseTheLanguagesComma(t *testing.T) {
	cases := []struct {
		locale i18n.Locale
		want   string
	}{
		{i18n.English, "Inbox, Sent and Trash"},
		{"es", "Inbox, Sent y Trash"},
		{"zh-cn", "Inbox、Sent 和 Trash"},
	}
	for _, c := range cases {
		ctx := i18n.WithLocale(context.Background(), c.locale)
		if got := settingsJoinAnd(ctx, []string{"Inbox", "Sent", "Trash"}); got != c.want {
			t.Errorf("%s: settingsJoinAnd = %q, want %q", c.locale, got, c.want)
		}
	}
	zh := i18n.WithLocale(context.Background(), "zh-cn")
	if got := settingsJoinAnd(zh, []string{"a", "b"}); got != "a 和 b" {
		t.Errorf("two names = %q", got)
	}
	if got := mailToAccountSources(zh, []string{"a", "b", "c", "d"}); got != "a、b、c 和 d" {
		t.Errorf("mailToAccountSources = %q", got)
	}
}
