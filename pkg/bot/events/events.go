package events

import (
	"context"
	"fmt"
	"sync"
)

type EventKind string
type CrimeResult string

const (
	AVAILABLESHIPS        EventKind = "available ships"
	BLINDJUMP             EventKind = "blind jump"
	BUSTED                EventKind = "busted"
	CORPPLANETLISTDISPLAY EventKind = "corp planet list display"
	DENSITYDISPLAY        EventKind = "density report display"
	DETONATORBUYMAX       EventKind = "detonator max to buy"
	FIGDEPLOY             EventKind = "fig deploy display"
	FIGHIT                EventKind = "fig hit"
	GTORPBUYMAX           EventKind = "gtorp max to buy"
	HOLDSTOBUY            EventKind = "holds to buy"
	MBOTNOTHINGTOSELL     EventKind = "MoM bot nothing to sell"
	MBOTTRADEDONE         EventKind = "MoM bot trade done"
	NOTVISITEDSECTORMSG   EventKind = "you have never visited sector"
	PLANETCREATE          EventKind = "planet create"
	PLANETDISPLAY         EventKind = "planet display"
	PLANETLANDINGDISPLAY  EventKind = "planet landing display"
	PLANETWARPCOMPLETE    EventKind = "planet warp complete"
	PORTEQUTOSTEAL        EventKind = "port equ to steal"
	PORTNOTINTERESTED     EventKind = "port not interested"
	PORTREPORTDISPLAY     EventKind = "port report"
	PORTROBCREDS          EventKind = "port rob creds"
	PROMPTDISPLAY         EventKind = "prompt display"
	QUICKSTATDISPLAY      EventKind = "quick stat display"
	ROBRESULT             EventKind = "rob result"
	ROUTEDISPLAY          EventKind = "route display"
	SECTORDISPLAY         EventKind = "sector display"
	SECTORWARPSDISPLAY    EventKind = "sector warps display"
	SHIPNOTAVAILABLE      EventKind = "ship not available for xport"
	STEALRESULT           EventKind = "steal result"
	TRADECOMPLETE         EventKind = "trade complete"
	TWARPLOCKED           EventKind = "twarp locked"
	TWARPLOWFUEL          EventKind = "twarp not enough fuel"
	TWXSCRIPTTERM         EventKind = "twx script terminated"
	WARPSINTOSECTOR       EventKind = "warps into sector"

	CRIMESUCCESS CrimeResult = "crime success"
	CRIMEABORT   CrimeResult = "crime abort"
	CRIMEBUSTED  CrimeResult = "crime busted"

	// prompts
	ATTACKPROMPT       = "attack prompt"
	BUYPROMPT          = "buy prompt"
	CITADELPROMPT      = "citadel prompt"
	COMMANDPROMPT      = "command prompt"
	COMPUTERPROMPT     = "computer prompt"
	CORPPROMPT         = "corp prompt"
	HWEMPORIUMPROMPT   = "hardware emporium prompt"
	MINEDSECTORPROMPT  = "mined sector prompt"
	MOMBOTPROMPT       = "MoM bot prompt"
	PLANETPROMPT       = "planet prompt"
	SELLPROMPT         = "sell prompt"
	SHIPYARDPROMPT     = "shipyard prompt"
	STARDOCKPROMPT     = "stardock prompt"
	STOPINSECTORPROMPT = "stop in this sector prompt"
)

type Event struct {
	Kind         EventKind
	ID           string
	Data         string
	DataInt      int
	DataSliceInt []int
}

type Wait struct {
	Kind EventKind
	ID   string
	c    chan<- *Event
}

type waitSlice []Wait

type waitMap map[string]waitSlice

func NewBroker() *Broker {
	return &Broker{
		listeners: map[EventKind][]func(*Event){},
		waits:     map[EventKind]waitMap{},
	}
}

type Broker struct {
	listenerLock sync.Mutex
	waitLock     sync.Mutex
	listeners    map[EventKind][]func(*Event)
	waits        map[EventKind]waitMap
}

func (b *Broker) Publish(e *Event) {
	fmt.Printf("Publishing event Kind: %s, ID: %s\n", e.Kind, e.ID)
	waits := b.getWaits(e.Kind, e.ID)

	if len(waits) > 0 {
		b.waitLock.Lock()
		for _, w := range waits {
			w.c <- e
			fmt.Printf("sent event to listener Kind: %s, ID: %s\n", w.Kind, w.ID)
		}
		b.waitLock.Unlock()
	}

	b.listenerLock.Lock()
	defer b.listenerLock.Unlock()

	listeners := b.listeners[e.Kind]
	for i, _ := range listeners {
		listeners[i](e)
	}
}

func (b *Broker) Subscribe(kind EventKind, callBack func(*Event)) {
	b.listenerLock.Lock()
	defer b.listenerLock.Unlock()

	listeners, ok := b.listeners[kind]
	if !ok {
		b.listeners[kind] = []func(*Event){callBack}
		return
	}
	b.listeners[kind] = append(listeners, callBack)
}

func (b *Broker) Waits() []Wait {
	ret := []Wait{}

	b.waitLock.Lock()
	defer b.waitLock.Unlock()

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

	b.waitLock.Lock()
	defer b.waitLock.Unlock()

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
	b.waitLock.Lock()
	defer b.waitLock.Unlock()

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
