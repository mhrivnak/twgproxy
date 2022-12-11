package events

import "context"

const (
	PLANETDISPLAY = "planet display"
	ROUTEDISPLAY  = "route display"
	SECTORDISPLAY = "sector display"
)

type Event struct {
	Kind string
	ID   string
	Data string
}

type wait struct {
	Kind string
	ID   string
	c    chan<- *Event
}

type waitMap map[string]wait

type Broker struct {
	waits map[string]waitMap
}

func (b *Broker) Publish(e *Event) {
	waits := b.getWaits(e.Kind, e.ID)
	for _, w := range waits {
		w.c <- e
	}
}

func (b *Broker) WaitFor(ctx context.Context, kind string, id string) <-chan *Event {
	if b.waits == nil {
		b.waits = map[string]waitMap{}
	}

	wm, ok := b.waits[kind]
	if !ok {
		wm = make(waitMap)
		b.waits[kind] = wm
	}

	// size 1 so an event can be sent even if the receiver is no longer waiting
	c := make(chan *Event, 1)

	wm[id] = wait{
		Kind: kind,
		ID:   id,
		c:    c,
	}

	return c
}

func (b *Broker) getWaits(kind string, id string) []wait {
	ret := []wait{}
	wm, ok := b.waits[kind]
	if !ok {
		return ret
	}

	w, ok := wm[id]
	if ok {
		ret = append(ret, w)
		delete(wm, id)
	}
	w, ok = wm[""]
	if ok {
		ret = append(ret, w)
		delete(wm, id)
	}
	return ret
}
