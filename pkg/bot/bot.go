package bot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/bot/parsers"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type Bot struct {
	gameReader    io.Reader
	userReader    io.Reader
	commandWriter io.Writer
	parsers       map[string]parsers.Parser
	data          *models.Data
	Actuator      actuator.Actuator
	Broker        *events.Broker
}

func New(game, user io.Reader, command io.Writer) *Bot {
	data := models.NewData()
	broker := &events.Broker{}

	return &Bot{
		gameReader:    game,
		userReader:    user,
		commandWriter: command,
		parsers:       map[string]parsers.Parser{},
		data:          data,
		Broker:        broker,
		Actuator:      *actuator.New(broker, data, command),
	}
}

// matches ANSI color codes
var ansiPattern *regexp.Regexp = regexp.MustCompile("\x1b\\[.*?m")

var warping *regexp.Regexp = regexp.MustCompile(`Warping to Sector (\d+)`)
var promptSector *regexp.Regexp = regexp.MustCompile(`\[([0-9]+)\] `)
var fighit *regexp.Regexp = regexp.MustCompile(`Deployed Fighters Report Sector (\d+)`)

func byteChan(r io.Reader) <-chan byte {
	c := make(chan byte)

	go func() {
		buf := bufio.NewReader(r)
		for {
			char, err := buf.ReadByte()
			fmt.Println(string(char))
			if err != nil {
				if err == io.EOF {
					fmt.Println("EOF from user client")
					return
				}
				fmt.Println(err.Error())
				continue
			}
			c <- char
		}
	}()
	return c
}

func (b *Bot) Start(done chan<- interface{}) {
	go func() {
		defer close(done)

		buf := bufio.NewReader(b.gameReader)
		var err error
		var line []byte
		var alreadyCheckedForPrompt bool
		for {
			line = make([]byte, 300)
		loop:
			for i := 0; i < 300; i++ {
				line[i], err = buf.ReadByte()
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				switch int(line[i]) {
				case 10: // \r
					b.ParseLine(string(line[:i]))
					break loop
				case 58: // :
					b.checkForFigHit(string(line[:i]))
				case 62: // >
					b.checkForMombotPrompt(string(line[:i+1]))
				case 63: // ?
					if alreadyCheckedForPrompt {
						continue
					}
					b.checkForPrompt(string(line[:i+1]))
					alreadyCheckedForPrompt = true
				}
			}
			alreadyCheckedForPrompt = false
		}

	}()

	go func() {
		defer close(done)

		// parse user input
		input := byteChan(b.userReader)

		data := []byte{}
		for {
			char := <-input
			switch int(char) {
			case 92: // "\"
				data = []byte{char}
			case 27: // ESC
				data = []byte{}
			case 8: // backspace
				if len(data) > 0 {
					data = data[:len(data)-1]
				}
			default:
				if len(data) > 0 {
					data = append(data, char)
					if bytes.ContainsAny([]byte{char}, "\n\r") {
						// parse the command and run an action if one is identified
						ctx, cancelCtx := context.WithCancel(context.Background())
						action := b.ParseCommand(ctx, data[1:len(data)-1])
						data = []byte{}
						if action != nil {
							done := action.Start(ctx)
						loop:
							for {
								select {
								case <-done:
									break loop
								case char = <-input:
									// if user pressed x, stop. Ignore other input.
									if char == []byte("x")[0] {
										cancelCtx()
										fmt.Println("cancelled action")
										break loop
									}
								}
							}
						}
					}
				} else {
					_, err := b.commandWriter.Write([]byte{char})
					if err != nil {
						if err == io.EOF {
							fmt.Println("lost connection to user client")
							return
						}
						fmt.Println(err.Error())
					}
				}
			}
		}
	}()
}

func (b *Bot) ParseCommand(ctx context.Context, command []byte) actions.Action {
	fmt.Printf("Parsing command: %s\n", string(command))

	if len(command) == 0 {
		return nil
	}

	switch command[0] {
	case []byte("p")[0]:
		if len(command) > 2 && command[1] == []byte("r")[0] {
			action, err := actions.NewPRouteTrade(string(command[2:]), &b.Actuator)
			if err != nil {
				fmt.Printf("failed to run planet route trade: %s\n", err.Error())
			}
			return action
		}
	case []byte("d")[0]:
		return actions.NewPDrop(&b.Actuator)
	case []byte("n")[0]:
		if len(command) == 2 {
			product, err := actions.ProductTypeFromChar(string(command[1]))
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			return actions.NewPTrade(0, product, &b.Actuator)
		}
		if len(command) > 2 {
			pid, err := strconv.Atoi(string(command[2:]))
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			product, err := actions.ProductTypeFromChar(string(command[1]))
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			return actions.NewPTrade(pid, product, &b.Actuator)
		}

	case []byte("m")[0]:
		dest, err := strconv.Atoi(string(command[1:]))
		if err != nil {
			fmt.Printf("failed to parse sector from command %s\n", string(command))
			return nil
		}
		return actions.NewMove(dest, &b.Actuator)
	case []byte("i")[0]:
		if len(command) == 1 {
			j, err := json.Marshal(b.data.Status)
			if err != nil {
				fmt.Println("failed to marshal Sector")
				return nil
			}
			fmt.Println(string(j))
			return nil
		}
		switch command[1] {
		case []byte("s")[0]:
			sector, err := strconv.Atoi(string(command[2:]))
			if err != nil {
				fmt.Printf("failed to parse sector from command %s\n", string(command))
				return nil
			}
			s, ok := b.data.Sectors[sector]
			if !ok {
				fmt.Printf("Don't have info on sector %d\n", sector)
				return nil
			}
			j, err := json.Marshal(s)
			if err != nil {
				fmt.Println("failed to marshal Sector")
				return nil
			}
			fmt.Println(string(j))
		case []byte("p")[0]:
			planetID, err := strconv.Atoi(string(command[2:]))
			if err != nil {
				fmt.Printf("failed to parse planet from command %s\n", string(command))
				return nil
			}
			planet, ok := b.data.Planets[planetID]
			if !ok {
				fmt.Printf("Don't have info on planet %d\n", planetID)
				return nil
			}
			j, err := json.Marshal(planet)
			if err != nil {
				fmt.Println("failed to marshal Planet")
				return nil
			}
			fmt.Println(string(j))
		}
	case []byte("r")[0]:
		if len(command) > 1 {
			fmt.Println("got extra args for rob command")
			return nil
		}
		return actions.NewRob(&b.Actuator)
	}
	return nil
}

func (b *Bot) ParseLine(line string) {
	clean := ansiPattern.ReplaceAllString(line, "")

	switch {
	case strings.HasPrefix(clean, "Warping to Sector"):
		sector, err := parseWarping(clean)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		b.data.Status.Sector = sector
	case strings.HasPrefix(clean, "Sector  : "):
		b.parsers[parsers.SECTORINFO] = parsers.NewSectorParser(b.data, b.Broker)
	case strings.HasPrefix(clean, "The shortest path ("):
		b.parsers[parsers.ROUTEINFO] = parsers.NewRouteParser(b.Broker)
	case strings.HasPrefix(clean, "Planet #"):
		b.parsers[parsers.PLANETINFO] = parsers.NewPlanetParser(b.data, b.Broker)
	case strings.HasPrefix(clean, "The Trade Journals estimate this port has"):
		b.parsers[parsers.PORTROBINFO] = parsers.NewPortRobParser(b.data, b.Broker)
	case strings.HasPrefix(clean, " Sect "):
		b.parsers[parsers.QUICKSTATS] = parsers.NewQuickStatsParser(b.data, b.Broker)
	case strings.HasPrefix(clean, "What sector is the port in?"):
		b.parsers[parsers.PORTREPORT] = parsers.NewPortReportParser(b.data, b.Broker)
	case strings.HasPrefix(clean, "Commerce report"):
		// a parser might already be in place if this is the result of a deliberate
		// port report being retrieved from a ship computer.
		_, ok := b.parsers[parsers.PORTREPORT]
		if !ok {
			b.parsers[parsers.PORTREPORT] = parsers.NewPortReportParser(b.data, b.Broker)
		}
	case strings.Contains(clean, "Corporate Planet Scan"):
		b.parsers[parsers.QUICKSTATS] = parsers.NewCorpPlanetsParser(b.data, b.Broker)
	case strings.Contains(clean, "[General] {cbot} - Done with port"):
		b.Broker.Publish(&events.Event{Kind: events.MBOTTRADEDONE})
	case strings.Contains(clean, "[General] {cbot} - Nothing to sell"):
		b.Broker.Publish(&events.Event{Kind: events.MBOTNOTHINGTOSELL})
	}

	for k, parser := range b.parsers {
		err := parser.Parse(clean)
		if err != nil {
			fmt.Println(err.Error())
		}
		if parser.Done() {
			delete(b.parsers, k)
		}
	}
}

func (b *Bot) checkForPrompt(line string) {
	clean := ansiPattern.ReplaceAllString(line, "")
	if len(clean) < 12 {
		return
	}

	e := events.Event{
		Kind: events.PROMPTDISPLAY,
	}

	switch clean[:12] {
	case "Command [TL=":
		e.ID = events.COMMANDPROMPT
		parts := promptSector.FindStringSubmatch(clean)
		if len(parts) != 2 {
			break
		}
		sector, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("failed to parse sector from prompt: %s\n", err.Error())
			break
		}
		b.data.Status.Sector = sector
	case "Planet comma":
		e.ID = events.PLANETPROMPT
	case "Computer com":
		e.ID = events.COMPUTERPROMPT
	case "Corporate co":
		e.ID = events.CORPPROMPT
	case "Citadel comm":
		e.ID = events.CITADELPROMPT
	case "<StarDock> W":
		e.ID = events.STARDOCKPROMPT
	case "<Shipyards> ":
		e.ID = events.SHIPYARDPROMPT
	}
	if e.ID != "" {
		b.Broker.Publish(&e)
	}
}

func (b *Bot) checkForMombotPrompt(line string) {
	clean := ansiPattern.ReplaceAllString(line, "")
	if strings.Contains(clean, "{General} cbot>") {
		b.Broker.Publish(&events.Event{
			Kind: events.PROMPTDISPLAY,
			ID:   events.MOMBOTPROMPT,
		})
	}
}

func (b *Bot) checkForFigHit(line string) {
	clean := ansiPattern.ReplaceAllString(line, "")
	if strings.Contains(clean, "Deployed Fighters Report Sector") {
		sector, err := parseFigHit(clean)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		b.Broker.Publish(&events.Event{
			Kind:    events.FIGHIT,
			DataInt: sector,
		})
	}
}

func parseWarping(line string) (int, error) {
	parts := warping.FindStringSubmatch(line)
	if len(parts) != 2 {
		return 0, fmt.Errorf("string match failed")
	}
	return strconv.Atoi(parts[1])
}

func parseFigHit(line string) (int, error) {
	parts := fighit.FindStringSubmatch(line)
	if len(parts) != 2 {
		fmt.Println(line)
		return 0, fmt.Errorf("string match failed")
	}
	return strconv.Atoi(parts[1])
}
