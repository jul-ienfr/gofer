package aievents

import (
	"testing"
	"time"
)

func TestBusScopesEventsToTheirUser(t *testing.T) {
	bus := NewBus()
	owner, cancelOwner := bus.Subscribe("owner")
	defer cancelOwner()
	other, cancelOther := bus.Subscribe("other")
	defer cancelOther()
	bus.Publish(Event{Type: "analysis.state", UserID: "owner", State: "done"})
	select {
	case event := <-owner:
		if event.State != "done" || event.At.IsZero() {
			t.Fatalf("owner event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("owner event not delivered")
	}
	select {
	case event := <-other:
		t.Fatalf("other user received %+v", event)
	default:
	}
}

func TestBusUnsubscribeStopsDelivery(t *testing.T) {
	bus := NewBus()
	events, cancel := bus.Subscribe("owner")
	cancel()
	bus.Publish(Event{UserID: "owner", Type: "analysis.state"})
	select {
	case event := <-events:
		t.Fatalf("event delivered after unsubscribe: %+v", event)
	default:
	}
	bus.Publish(Event{UserID: "owner", Type: "analysis.state"})
}
