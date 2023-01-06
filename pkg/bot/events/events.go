package events

import (
	"context"
	"fmt"
	"sync"
)

type EventKind string

const (
	CORPPLANETLISTDISPLAY EventKind = "corp planet list display"
	DENSITYDISPLAY        EventKind = "density report display"
	FIGHIT                EventKind = "fig hit"
	MBOTNOTHINGTOSELL     EventKind = "MoM bot nothing to sell"
	MBOTTRADEDONE         EventKind = "MoM bot trade done"
	PLANETDISPLAY         EventKind = "planet display"
	PORTREPORTDISPLAY     EventKind = "port report"
	PORTROBCREDS          EventKind = "port rob creds"
	PROMPTDISPLAY         EventKind = "prompt display"
	ROUTEDISPLAY          EventKind = "route display"
	SECTORDISPLAY         EventKind = "sector display"

	// prompts
	CITADELPROMPT    = "citadel prompt"
	COMMANDPROMPT    = "command prompt"
	COMPUTERPROMPT   = "computer prompt"
	CORPPROMPT       = "corp prompt"
	HWEMPORIUMPROMPT = "hardware emporium prompt"
	MOMBOTPROMPT     = "MoM bot prompt"
	PLANETPROMPT     = "planet prompt"
	SHIPYARDPROMPT   = "shipyard prompt"
	STARDOCKPROMPT   = "stardock prompt"
)

type Event struct {
	Kind    EventKind
	ID      string
	Data    string
	DataInt int
}

type Wait struct {
	Kind EventKind
	ID   string
	c    chan<- *Event
}

type waitSlice []Wait

type waitMap map[string]waitSlice

type Broker struct {
	sync.Mutex
	waits map[EventKind]waitMap
}

func (b *Broker) Publish(e *Event) {
	fmt.Printf("Publishing event Kind: %s, ID: %s\n", e.Kind, e.ID)
	waits := b.getWaits(e.Kind, e.ID)

	if len(waits) > 0 {
		b.Lock()
		defer b.Unlock()
	}

	for _, w := range waits {
		w.c <- e
		fmt.Printf("sent event to listener Kind: %s, ID: %s\n", w.Kind, w.ID)
	}
}

func (b *Broker) Waits() []Wait {
	ret := []Wait{}

	b.Lock()
	defer b.Unlock()

	for _, wm := range b.waits {
		for _, w := range wm {
			ret = append(ret, w...)
		}
	}

	return ret
}

func (b *Broker) WaitFor(ctx context.Context, kind EventKind, id string) <-chan *Event {
	if b.waits == nil {
		b.waits = map[EventKind]waitMap{}
	}

	b.Lock()
	defer b.Unlock()

	wm, ok := b.waits[kind]
	if !ok {
		wm = make(waitMap)
		b.waits[kind] = wm
	}

	// size 1 so an event can be sent even if the receiver is no longer waiting
	c := make(chan *Event, 1)

	wm[id] = append(wm[id], Wait{
		Kind: kind,
		ID:   id,
		c:    c,
	})

	return c
}

func (b *Broker) getWaits(kind EventKind, id string) []Wait {
	ret := waitSlice{}
	b.Lock()
	defer b.Unlock()

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
