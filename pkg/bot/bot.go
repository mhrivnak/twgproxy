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
	Broker        *events.Broker
}

func New(game, user io.Reader, command io.Writer) *Bot {
	return &Bot{
		gameReader:    game,
		userReader:    user,
		commandWriter: command,
		parsers:       map[string]parsers.Parser{},
		data:          models.NewData(),
		Broker:        &events.Broker{},
	}
}

// matches ANSI color codes
var ansiPattern *regexp.Regexp = regexp.MustCompile("\x1b\\[.*?m")

var warping *regexp.Regexp = regexp.MustCompile(`Warping to Sector (\d+)`)
var promptSector *regexp.Regexp = regexp.MustCompile(`\[([0-9]+)\] `)

func byteChan(r io.Reader) <-chan byte {
	c := make(chan byte)

	go func() {
		buf := bufio.NewReader(r)
		for {
			char, err := buf.ReadByte()
			fmt.Println(string(char))
			if err != nil {
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
			line = make([]byte, 200)
		loop:
			for i := 0; i < 200; i++ {
				line[i], err = buf.ReadByte()
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				switch int(line[i]) {
				case 10: // \n
					b.ParseLine(string(line[:i]))
					break loop
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
						fmt.Println(err.Error())
					}
				}
			}
		}
	}()
}

func (b *Bot) SendCommand(command string) error {
	_, err := b.commandWriter.Write([]byte(command))
	return err
}

func (b *Bot) ParseCommand(ctx context.Context, command []byte) actions.Action {
	fmt.Printf("Parsing command: %s\n", string(command))

	if len(command) == 0 {
		return nil
	}

	switch command[0] {
	case []byte("m")[0]:
		dest, err := strconv.Atoi(string(command[1:]))
		if err != nil {
			fmt.Printf("failed to parse sector from command %s\n", string(command))
			return nil
		}
		return actions.NewMove(dest, b.Broker, b.data, b.SendCommand)
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
		return actions.NewRob(b.Broker, b.data, b.SendCommand)
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

func parseWarping(line string) (int, error) {
	parts := warping.FindStringSubmatch(line)
	if len(parts) != 2 {
		return 0, fmt.Errorf("string match failed")
	}
	return strconv.Atoi(parts[1])
}
