package views

import (
	"strings"
	"testing"
)

func TestSlideIndicatorTransformMirrorsRTL(t *testing.T) {
	tests := []struct {
		name      string
		step      int
		slots     int
		direction string
		want      string
	}{
		{name: "first ltr", step: 0, slots: 2, direction: "ltr", want: "translateX(0)"},
		{name: "second ltr", step: 1, slots: 2, direction: "ltr", want: "translateX(calc(100% + 2px))"},
		{name: "second rtl", step: 1, slots: 2, direction: "rtl", want: "translateX(0)"},
		{name: "first rtl", step: 0, slots: 2, direction: "rtl", want: "translateX(calc(100% + 2px))"},
		{name: "third ltr", step: 2, slots: 3, direction: "ltr", want: "translateX(calc(200% + 4px))"},
		{name: "third rtl", step: 2, slots: 3, direction: "rtl", want: "translateX(0)"},
		{name: "first of three rtl", step: 0, slots: 3, direction: "rtl", want: "translateX(calc(200% + 4px))"},
		{name: "out of range", step: 3, slots: 3, direction: "rtl", want: "translateX(0)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := slideIndicatorTransform(test.step, test.slots, test.direction); got != test.want {
				t.Errorf("slideIndicatorTransform(%d, %d, %q) = %q, want %q", test.step, test.slots, test.direction, got, test.want)
			}
		})
	}
}

func TestIndicatorStylesFollowDirection(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "mail cards rtl", got: mailListViewIndicatorStyle("cards", "rtl"), want: "translateX(calc(100% + 2px))"},
		{name: "mail table rtl", got: mailListViewIndicatorStyle("table", "rtl"), want: "translateX(0)"},
		{name: "contacts cards rtl", got: contactViewIndicatorStyle("cards", "rtl"), want: "translateX(calc(100% + 2px))"},
		{name: "contacts table rtl", got: contactViewIndicatorStyle("table", "rtl"), want: "translateX(0)"},
		{name: "calendar month rtl", got: calendarViewIndicatorStyle("month", "rtl"), want: "translateX(calc(100% + 2px))"},
		{name: "calendar week rtl", got: calendarViewIndicatorStyle("week", "rtl"), want: "translateX(0)"},
		{name: "sidebar mail rtl", got: sidebarAppIndicatorStyle("mail", "rtl"), want: "translateX(calc(200% + 4px))"},
		{name: "sidebar contacts rtl", got: sidebarAppIndicatorStyle("contacts", "rtl"), want: "translateX(calc(100% + 2px))"},
		{name: "sidebar calendar rtl", got: sidebarAppIndicatorStyle("calendar", "rtl"), want: "translateX(0)"},
		{name: "mail cards ltr", got: mailListViewIndicatorStyle("cards", "ltr"), want: "translateX(0)"},
		{name: "contacts table ltr", got: contactViewIndicatorStyle("table", "ltr"), want: "translateX(calc(100% + 2px))"},
		{name: "calendar week ltr", got: calendarViewIndicatorStyle("week", "ltr"), want: "translateX(calc(100% + 2px))"},
		{name: "sidebar calendar ltr", got: sidebarAppIndicatorStyle("calendar", "ltr"), want: "translateX(calc(200% + 4px))"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(test.got, test.want) {
				t.Errorf("indicator style %q does not contain %q", test.got, test.want)
			}
		})
	}
}
