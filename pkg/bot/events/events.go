package events

import (
	"context"
	"fmt"
	"sync"

	localcontext "github.com/mhrivnak/twgproxy/pkg/bot/context"
)

type EventKind string
type CrimeResult string

const (
	AVAILABLESHIPS        EventKind = "available ships"
	BLINDJUMP             EventKind = "blind jump"
	BUSTED                EventKind = "busted"
	CONFIGDISPLAY         EventKind = "config display"
	CORPPLANETLISTDISPLAY EventKind = "corp planet list display"
	DENSITYDISPLAY        EventKind = "density report display"
	DETONATORBUYMAX       EventKind = "detonator max to buy"
	FIGDEPLOY             EventKind = "fig deploy display"
	FIGSDESTROYED         EventKind = "figs destroyed"
	FIGHIT                EventKind = "fig hit"
	FIGSTOBUY             EventKind = "figs to buy"
	GTORPBUYMAX           EventKind = "gtorp max to buy"
	HOLDSTOBUY            EventKind = "holds to buy"
	MBOTNOTHINGTOSELL     EventKind = "MoM bot nothing to sell"
	MBOTTRADEDONE         EventKind = "MoM bot trade done"
	MINESDESTROYED        EventKind = "mines destroyed"
	MINESALLDESTROYED     EventKind = "mines all destroyed"
	NAVHAZ                EventKind = "nav haz"
	NOTVISITEDSECTORMSG   EventKind = "you have never visited sector"
	PLANETCREATE          EventKind = "planet create"
	PLANETDISPLAY         EventKind = "planet display"
	PLANETLANDINGDISPLAY  EventKind = "planet landing display"
	PLANETWARPCOMPLETE    EventKind = "planet warp complete"
	PORTEQUTOSTEAL        EventKind = "port equ to steal"
	PORTNOINFO            EventKind = "no info about a port in that sector"
	PORTNOTINTERESTED     EventKind = "port not interested"
	PORTREPORTDISPLAY     EventKind = "port report"
	PORTROBCREDS          EventKind = "port rob creds"
	PROMPTDISPLAY         EventKind = "prompt display"
	QUICKSTATDISPLAY      EventKind = "quick stat display"
	ROBRESULT             EventKind = "rob result"
	ROUTEDISPLAY          EventKind = "route display"
	SECTORDISPLAY         EventKind = "sector display"
	SECTORWARPSDISPLAY    EventKind = "sector warps display"
	SHIELDSTOBUY          EventKind = "shields to buy"
	SHIELDSABSORBEDATTACK EventKind = "shields absorbed the attack"
	SHIPNOTAVAILABLE      EventKind = "ship not available for xport"
	STEALRESULT           EventKind = "steal result"
	TWARPLOCKED           EventKind = "twarp locked"
	TWARPLOWFUEL          EventKind = "twarp not enough fuel"
	TWARPPOWERTYPE1       EventKind = "twarp power type 1"
	TWARPPOWERTYPE2       EventKind = "twarp power type 2"
	TWXSCRIPTTERM         EventKind = "twx script terminated"
	WARPSINTOSECTOR       EventKind = "warps into sector"
	XPORTRANGE            EventKind = "xport range"
	YOUHAVECREDS          EventKind = "you have creds"

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

// waitMap groups waits by their ID
type waitMap map[string]waitSlice

func NewBroker() *Broker {
	return &Broker{
		listeners: map[EventKind][]func(*Event){},
		waits:     map[string]map[EventKind]waitMap{},
	}
}

type Broker struct {
	listenerLock sync.Mutex
	waitLock     sync.Mutex
	listeners    map[EventKind][]func(*Event)

	// waits are organized first as a map of group ID, which is an ID pulled off
	// of the context when a wait is created. That makes it easy to prune all of
	// the leftover waits when a section of code no longer needs them. The second
	// map groups waits by their event kind.
	waits map[string]map[EventKind]waitMap
}

// prune removes all waits that were created with the given context
func (b *Broker) prune(ctx context.Context) {
	b.waitLock.Lock()
	defer b.waitLock.Unlock()

	groupID := localcontext.IDFromContext(ctx)
	if groupID != "" {
		fmt.Printf("pruning wait group: %s\n", groupID)
		delete(b.waits, groupID)
	}
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

	for _, group := range b.waits {
		for _, wm := range group {
			for _, w := range wm {
				ret = append(ret, w...)
			}
		}
	}

	return ret
}

func (b *Broker) WaitFor(ctx context.Context, kind EventKind, id string) <-chan *Event {
	if b.waits == nil {
		b.waits = map[string]map[EventKind]waitMap{}
	}

	groupID := localcontext.IDFromContext(ctx)

	b.waitLock.Lock()
	defer b.waitLock.Unlock()

	group, ok := b.waits[groupID]
	if !ok {
		b.waits[groupID] = map[EventKind]waitMap{}
		group = b.waits[groupID]
		if groupID != "" {
			// once the context is cancelled, prune the wait group
			go func() {
				<-ctx.Done()
				b.prune(ctx)
			}()
		}
	}
	wm, ok := group[kind]
	if !ok {
		wm = make(waitMap)
		group[kind] = wm
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

	for _, group := range b.waits {
		wm, ok := group[kind]
		if !ok {
			continue
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
	}

	return ret
}
