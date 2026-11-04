package views

import "github.com/cristianadrielbraun/gofer/internal/i18n"

func uiLocale(settings map[string]string) i18n.Locale {
	return i18n.Resolve(settings["language"])
}

func uiLang(settings map[string]string) string {
	return uiLocale(settings).String()
}

func tr(settings map[string]string, key string) string {
	return i18n.T(uiLocale(settings), key)
}
