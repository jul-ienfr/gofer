package i18n

import (
	"fmt"
	"strings"
)

// French is the fork's translated catalog. Upstream discovers locales from
// locales/*.json; the fork-only features still read these Go catalogs.
const French Locale = "fr"

var legacyMessages = map[Locale]map[string]string{
	English: {},
	French:  {},
}

func init() {
	for key, value := range english {
		legacyMessages[English][key] = value
	}
	for key, value := range french {
		legacyMessages[French][key] = value
	}
	for key, value := range loginEnglish {
		legacyMessages[English][key] = value
	}
	for key, value := range loginFrench {
		legacyMessages[French][key] = value
	}
}

// L returns a message from the fork's Go catalogs.
func L(locale Locale, key string) string {
	if catalog, ok := legacyMessages[locale]; ok {
		if value, ok := catalog[key]; ok {
			return value
		}
	}
	if value, ok := legacyMessages[English][key]; ok {
		return value
	}
	return key
}

// Lf formats a fork catalog message with fmt.Sprintf when arguments are given.
func Lf(locale Locale, key string, args ...any) string {
	value := L(locale, key)
	if len(args) == 0 {
		return value
	}
	return fmt.Sprintf(value, args...)
}

// LPlural returns "one" or "other" for the locale's cardinal rule.
func LPlural(locale Locale, count int) string {
	if locale == French && (count == 0 || count == 1) {
		return "one"
	}
	if count == 1 {
		return "one"
	}
	return "other"
}

// LTP picks the singular or plural key for count and formats it.
func LTP(locale Locale, count int, one, other string, args ...any) string {
	key := other
	if LPlural(locale, count) == "one" {
		key = one
	}
	return Lf(locale, key, args...)
}

// Normalize maps a language tag such as fr-CA or fr_CA to a supported locale.
func Normalize(raw string) (Locale, bool) {
	tag := strings.ToLower(strings.TrimSpace(raw))
	tag = strings.ReplaceAll(tag, "_", "-")
	if index := strings.IndexByte(tag, '-'); index >= 0 {
		tag = tag[:index]
	}
	switch Locale(tag) {
	case English:
		return English, true
	case French:
		return French, true
	default:
		return English, false
	}
}

// Resolve returns the matching fork locale, or English for an unknown tag.
func Resolve(raw string) Locale {
	if locale, ok := Normalize(raw); ok {
		return locale
	}
	return English
}

// Supported lists the locales the fork catalogs offer.
func Supported() []Locale {
	return []Locale{English, French}
}
