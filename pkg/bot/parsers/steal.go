package parsers

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewParseSteal(broker *events.Broker) Parser {
	return &ParseSteal{
		broker: broker,
	}
}

type ParseSteal struct {
	broker *events.Broker
	done   bool
}

var equOnDock *regexp.Regexp = regexp.MustCompile(`^Equipment  Buying +[0-9]+ +([0-9]+) `)

func (p *ParseSteal) Done() bool {
	return p.done
}

func (p *ParseSteal) Parse(line string) error {
	parts := equOnDock.FindStringSubmatch(line)
	if len(parts) == 2 {
		equ, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("could not parse equ on dock: %s\n", err.Error())
			p.done = true
			return err
		}

		p.broker.Publish(&events.Event{
			Kind:    events.PORTEQUTOSTEAL,
			DataInt: equ,
		})

		p.done = true
	}

	return nil
}
