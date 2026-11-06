// Package i18n holds Gofer's interface catalogs and locale rules.
package i18n

import (
	"fmt"
	"strings"
)

// Locale identifies an interface language catalog.
type Locale string

const (
	// English is the source catalog and the fallback for every lookup.
	English Locale = "en"
	// French is the first translated catalog.
	French Locale = "fr"
)

var messages = map[Locale]map[string]string{
	English: english,
	French:  french,
}

func (l Locale) String() string {
	return string(l)
}

// Supported lists the locales offered in the interface language picker.
func Supported() []Locale {
	return []Locale{English, French}
}

// NativeName returns the language name shown in its own language.
func NativeName(locale Locale) string {
	switch locale {
	case French:
		return "Français"
	default:
		return "English"
	}
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

// Resolve returns the matching locale, or English for an unknown tag.
func Resolve(raw string) Locale {
	if locale, ok := Normalize(raw); ok {
		return locale
	}
	return English
}

// Match returns the first supported locale named in an Accept-Language
// header. Browsers list their preferences in order; unsupported tags are
// skipped.
func Match(header string) Locale {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if locale, ok := Normalize(tag); ok {
			return locale
		}
	}
	return English
}

// T returns the translation for key, falling back to English and then the key.
func T(locale Locale, key string) string {
	return text(locale, key)
}

// Tf formats a translated message with fmt.Sprintf when arguments are given.
func Tf(locale Locale, key string, args ...any) string {
	value := text(locale, key)
	if len(args) == 0 {
		return value
	}
	return fmt.Sprintf(value, args...)
}

// Plural returns "one" or "other" for the locale's cardinal rule.
func Plural(locale Locale, count int) string {
	if locale == French {
		if count == 0 || count == 1 {
			return "one"
		}
		return "other"
	}
	if count == 1 {
		return "one"
	}
	return "other"
}

// TP picks the singular or plural key for count and formats it.
func TP(locale Locale, count int, one, other string, args ...any) string {
	key := other
	if Plural(locale, count) == "one" {
		key = one
	}
	return Tf(locale, key, args...)
}

func text(locale Locale, key string) string {
	if catalog, ok := messages[locale]; ok {
		if value, ok := catalog[key]; ok {
			return value
		}
	}
	if value, ok := messages[English][key]; ok {
		return value
	}
	return key
}
