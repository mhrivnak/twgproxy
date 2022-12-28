package events

import (
	"context"
	"fmt"
)

type EventKind string

const (
	PLANETDISPLAY         EventKind = "planet display"
	PORTROBCREDS          EventKind = "port rob creds"
	ROUTEDISPLAY          EventKind = "route display"
	SECTORDISPLAY         EventKind = "sector display"
	PROMPTDISPLAY         EventKind = "prompt display"
	CORPPLANETLISTDISPLAY EventKind = "corp planet list display"
	MBOTTRADEDONE         EventKind = "MoM bot trade done"
	MBOTNOTHINGTOSELL     EventKind = "MoM bot nothing to sell"
	FIGHIT                EventKind = "fig hit"
	PORTREPORTDISPLAY     EventKind = "port report"

	// prompts
	COMMANDPROMPT    = "command prompt"
	COMPUTERPROMPT   = "computer prompt"
	PLANETPROMPT     = "planet prompt"
	CORPPROMPT       = "corp prompt"
	CITADELPROMPT    = "citadel prompt"
	STARDOCKPROMPT   = "stardock prompt"
	SHIPYARDPROMPT   = "shipyard prompt"
	HWEMPORIUMPROMPT = "hardware emporium prompt"
	MOMBOTPROMPT     = "MoM bot prompt"
)

type Event struct {
	Kind    EventKind
	ID      string
	Data    string
	DataInt int
}

type wait struct {
	Kind EventKind
	ID   string
	c    chan<- *Event
}

type waitSlice []wait

type waitMap map[string]waitSlice

type Broker struct {
	waits map[EventKind]waitMap
}

func (b *Broker) Publish(e *Event) {
	fmt.Printf("Publishing event Kind: %s, ID: %s\n", e.Kind, e.ID)
	waits := b.getWaits(e.Kind, e.ID)
	for _, w := range waits {
		w.c <- e
		fmt.Printf("sent event to listener Kind: %s, ID: %s\n", w.Kind, w.ID)
	}
}

func (b *Broker) WaitFor(ctx context.Context, kind EventKind, id string) <-chan *Event {
	if b.waits == nil {
		b.waits = map[EventKind]waitMap{}
	}

	wm, ok := b.waits[kind]
	if !ok {
		wm = make(waitMap)
		b.waits[kind] = wm
	}

	// size 1 so an event can be sent even if the receiver is no longer waiting
	c := make(chan *Event, 1)

	wm[id] = append(wm[id], wait{
		Kind: kind,
		ID:   id,
		c:    c,
	})

	return c
}

func (b *Broker) getWaits(kind EventKind, id string) []wait {
	ret := waitSlice{}
	wm, ok := b.waits[kind]
	if !ok {
		return ret
	}

	wSlice, ok := wm[id]
	if ok {
		ret = append(ret, wSlice...)
		delete(wm, id)
	}
	globalWaitSlice, ok := wm[""]
	if ok {
		ret = append(ret, globalWaitSlice...)
		delete(wm, "")
	}

	return ret
}
