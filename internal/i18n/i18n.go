// Package i18n holds Gofer's interface catalogs and locale rules.
package i18n

import (
	"fmt"
	"strings"
)

type Locale string

const (
	English Locale = "en"
	French  Locale = "fr"
)

var messages = map[Locale]map[string]string{
	English: english,
	French:  french,
}

func (l Locale) String() string {
	return string(l)
}

func Supported() []Locale {
	return []Locale{English, French}
}

func NativeName(locale Locale) string {
	switch locale {
	case French:
		return "Français"
	default:
		return "English"
	}
}

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

func Resolve(raw string) Locale {
	if locale, ok := Normalize(raw); ok {
		return locale
	}
	return English
}

func T(locale Locale, key string) string {
	return text(locale, key)
}

func Tf(locale Locale, key string, args ...any) string {
	value := text(locale, key)
	if len(args) == 0 {
		return value
	}
	return fmt.Sprintf(value, args...)
}

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
