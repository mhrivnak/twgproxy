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
	"github.com/mhrivnak/twgproxy/pkg/bot/listeners"
	"github.com/mhrivnak/twgproxy/pkg/bot/parsers"
	"github.com/mhrivnak/twgproxy/pkg/models"
	"gorm.io/gorm"
)

type Bot struct {
	gameReader    io.Reader
	commandWriter io.Writer
	parsers       map[string]parsers.Parser
	data          *models.Data
	Actuator      *actuator.Actuator
	Broker        *events.Broker
	db            *gorm.DB
}

func New(game io.Reader, command io.Writer, db *gorm.DB) *Bot {
	data := models.NewData(db)
	broker := events.NewBroker()

	return &Bot{
		gameReader:    game,
		commandWriter: command,
		parsers:       map[string]parsers.Parser{},
		data:          data,
		Broker:        broker,
		Actuator:      actuator.New(broker, data, command),
		db:            db,
	}
}

// matches ANSI color codes
var ansiPattern *regexp.Regexp = regexp.MustCompile("\x1b\\[.*?m")

var warping *regexp.Regexp = regexp.MustCompile(`Warping to Sector (\d+)`)
var promptSector *regexp.Regexp = regexp.MustCompile(`\[([0-9]+)\] `)
var fighit *regexp.Regexp = regexp.MustCompile(`Deployed Fighters Report Sector (\d+)`)
var promptBuySell *regexp.Regexp = regexp.MustCompile(`How many holds of ([ a-zA-Z]+) do you want to ([a-z]+) \[`)
var tradeComplete *regexp.Regexp = regexp.MustCompile(`You have [0-9,]+ credits and ([0-9]+) empty cargo holds.`)
var holdsToBuy *regexp.Regexp = regexp.MustCompile(`^A  Cargo holds +: +[0-9]+ credits / next hold +([0-9]+)`)
var figsToBuy *regexp.Regexp = regexp.MustCompile(`^B  Fighters +: +[0-9]+ credits per fighter +([0-9]+)`)
var shieldsToBuy *regexp.Regexp = regexp.MustCompile(`^C  Shield Points +: +[0-9]+ credits per point +([0-9]+)`)

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
					close(c)
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

func (b *Bot) Start(userReader io.Reader, done chan<- interface{}) {
	// Setup listeners
	b.Broker.Subscribe(events.BUSTED, listeners.NewBustHandler(b.Actuator))
	b.Broker.Subscribe(events.SECTORDISPLAY, listeners.NewSectorHandler(b.Actuator))

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
					//fmt.Println(string(line[:i+1]))
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
		input := byteChan(userReader)

		data := []byte{}
		for char := range input {
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
						action := b.ParseCommand(data[1 : len(data)-1])
						data = []byte{}
						if action != nil {
							b.runAction(action, input)
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

// runAction runs the action until it completes, unless the user cancels it by
// pressing "x".
func (b *Bot) runAction(action actions.Action, input <-chan byte) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	actionDone := action.Start(ctx)
	for {
		select {
		case <-actionDone:
			return
		case char := <-input:
			// if user pressed x, stop.
			if char == byte('x') {
				fmt.Println("cancelled action")
				return
			}
			// if user pressed ?, print what we're waiting on
			if char == byte('?') {
				for _, w := range b.Broker.Waits() {
					fmt.Printf("waiting on Kind: %s  ID: %s\n", w.Kind, w.ID)
				}
			}
		}
	}
}

func (b *Bot) ParseCommand(command []byte) actions.Action {
	fmt.Printf("Parsing command: %s\n", string(command))

	if len(command) == 0 {
		return nil
	}

	switch command[0] {
	case byte('w'):
		if string(command[:4]) == "wppt" {
			return actions.NewWPPT(b.Actuator)
		}
		if string(command[:4]) == "wsst" {
			shipID, err := strconv.Atoi(string(command[4:]))
			if err != nil {
				fmt.Printf("error parsing wsst ship: %s", err.Error())
				return nil
			}
			return actions.NewWSST(b.Actuator, shipID)
		}
	case byte('c'):
		if len(command) > 1 {
			switch command[1] {
			case byte('w'):
				b.parsers[parsers.CIMWARPS] = parsers.NewCIMWarpsParser(b.data.Persist.WarpCache)
				return actions.NewCIMWarpUpdate(b.Actuator)
			}
		}
	case byte('a'):
		if len(command) > 1 {
			switch command[1] {
			case byte('u'):
				return actions.NewUnsurround(b.Actuator)
			case byte('s'):
				figs, err := strconv.Atoi(string(command[2:]))
				if err != nil {
					fmt.Printf("failed to parse number of figs: %s", err.Error())
					return nil
				}
				return actions.NewSurround(figs, b.Actuator)
			}
		}
	case byte('p'):
		if len(command) > 2 && command[1] == byte('s') {
			fromID, toID, err := parsePStripArgs(string(command[2:]))
			if err != nil {
				fmt.Printf("failed to parse pstrip args: %s\n", err.Error())
				return nil
			}
			// bulk pstrip creates and destroys planets to strip
			if fromID == 0 {
				return actions.NewPStripBulk(toID, b.Actuator)
			}
			return actions.NewPStrip(fromID, toID, b.Actuator)
		}
		if len(command) > 2 && command[1] == byte('r') {
			action, err := actions.NewPRouteTrade(string(command[2:]), b.Actuator)
			if err != nil {
				fmt.Printf("failed to run planet route trade: %s\n", err.Error())
				return nil
			}
			return action
		}
		if len(command) > 2 && command[1] == byte('w') {
			action, err := actions.NewPWarpSell(string(command[2:]), b.Actuator)
			if err != nil {
				fmt.Printf("failed to run planet route trade: %s\n", err.Error())
				return nil
			}
			return action
		}
		if len(command) > 2 && command[1] == byte('c') {
			args, err := parsePCreateArgs(string(command[2:]))
			if err != nil {
				fmt.Printf("failed to parse planet create args: %s\n", err.Error())
				return nil
			}
			return actions.NewPCreate(args, b.Actuator)
		}
		if len(command) > 3 && string(command[1:3]) == "fd" {
			figs, err := strconv.Atoi(string(command[3:]))
			if err != nil {
				fmt.Printf("failed to parse fig count from args: %s\n", err.Error())
				return nil
			}
			return actions.NewPFigDeploy(figs, b.Actuator)
		}
		if len(command) > 2 && command[1] == byte('u') {
			upgrade, err := actions.NewPUpgrade(string(command[2:]), b.Actuator)
			if err != nil {
				fmt.Printf("failed to run mass upgrade route: %s\n", err.Error())
				return nil
			}
			return upgrade
		}
	case byte('d'):
		return actions.NewPDrop(b.Actuator)
	case byte('n'):
		if len(command) == 2 {
			product, err := models.ProductTypeFromChar(string(command[1]))
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			return actions.NewPTrade(0, product, b.Actuator)
		}
		if len(command) > 2 {
			pid, err := strconv.Atoi(string(command[2:]))
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			product, err := models.ProductTypeFromChar(string(command[1]))
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			return actions.NewPTrade(pid, product, b.Actuator)
		}

	case byte('m'):
		if len(command) > 1 && command[1] == byte('f') {
			dest, err := strconv.Atoi(string(command[2:]))
			if err != nil {
				fmt.Printf("failed to parse sector from command %s\n", string(command))
				return nil
			}
			opts := actuator.MoveOptions{
				DropFigs:      1,
				MinFigs:       5000,
				EnemyFigsMax:  1000,
				EnemyMinesMax: 50,
			}
			return actions.NewMove(dest, opts, b.Actuator)
		}
		dest, err := strconv.Atoi(string(command[1:]))
		if err != nil {
			fmt.Printf("failed to parse sector from command %s\n", string(command))
			return nil
		}
		return actions.NewMove(dest, actuator.MoveOptions{}, b.Actuator)
	case byte('e'):
		dest, err := strconv.Atoi(string(command[1:]))
		if err != nil {
			fmt.Printf("failed to parse sector from command %s\n", string(command))
			return nil
		}
		return actions.NewExplore(dest, b.Actuator)
	case byte('i'):
		if len(command) == 1 {
			j, err := json.Marshal(b.data.Status)
			if err != nil {
				fmt.Println("failed to marshal Status")
				return nil
			}
			fmt.Println(string(j))
			return nil
		}
		switch command[1] {
		case byte('s'):
			sector, err := strconv.Atoi(string(command[2:]))
			if err != nil {
				fmt.Printf("failed to parse sector from command %s\n", string(command))
				return nil
			}
			s, ok := b.data.GetSector(sector)
			if !ok {
				fmt.Printf("Don't have info on sector %d\n", sector)
				return nil
			}
			j, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				fmt.Println("failed to marshal Sector")
				return nil
			}
			fmt.Println(string(j))
		case byte('p'):
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
	case byte('r'):
		if len(command) == 1 {
			return actions.NewRob(b.Actuator)
		}
		if len(command) > 2 && command[1] == byte('p') {
			// rob pair
			otherPort, err := strconv.Atoi((string(command[2:])))
			if err != nil {
				fmt.Printf("failed to parse other sector from arg: %s\n", err.Error())
				return nil
			}
			return actions.NewRobPair(otherPort, b.Actuator)
		}
	case byte('s'):
		if len(command) == 1 {
			j, err := json.Marshal(b.data.Settings)
			if err != nil {
				fmt.Println("failed to marshal Settings")
				return nil
			}
			fmt.Println(string(j))
			return nil
		}
		switch command[1] {
		case byte('t'):
			parts := strings.Split(string(command[2:]), ",")
			if len(parts) == 1 {
				b.data.Settings.HopsToSD = []models.TwarpHop{}
				return nil
			}
			if len(parts)%2 != 0 {
				fmt.Println("did not get an even number of arguments")
				return nil
			}
			hops := []models.TwarpHop{}
			hop := models.TwarpHop{}
			for i, arg := range parts {
				argInt, err := strconv.Atoi(arg)
				if err != nil {
					fmt.Println(err.Error())
					return nil
				}

				if i%2 == 0 {
					hop = models.TwarpHop{
						Sector: argInt,
					}
				} else {
					hop.Planet = argInt
					hops = append(hops, hop)
				}
			}
			b.data.Settings.HopsToSD = hops
			return nil
		}
	case byte('g'):
		switch command[1] {
		case byte('s'):
			return actions.WrapErr(b.Actuator.GoToSD)
		}
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
	case strings.Contains(clean, "Relative Density Scan"):
		b.parsers[parsers.DENSITYSCAN] = parsers.NewParseDensityScan(b.data, b.Broker)
	case strings.HasPrefix(clean, "What do you want to name this planet?"):
		b.parsers[parsers.PLANETCREATE] = parsers.NewPCreateParser(b.Broker)
	case strings.HasPrefix(clean, "Registry# and Planet Name"):
		b.parsers[parsers.PLANETLANDING] = parsers.NewPlanetLandingParser(b.Broker)
	case strings.HasPrefix(clean, "<Drop/Take Fighters>"):
		b.parsers[parsers.FIGDEPLOY] = parsers.NewFigDeployParser(b.Broker)
	case strings.HasPrefix(clean, "You connect to their control computer to siphon the funds out"):
		b.parsers[parsers.ROBRESULT] = parsers.NewRobResultParser(b.Broker, b.data.Status.Sector)
	case strings.HasPrefix(clean, "You start your droids loading the cargo"):
		b.parsers[parsers.ROBRESULT] = parsers.NewStealResultParser(b.Broker, b.data.Status.Sector)
	case strings.HasPrefix(clean, "Script terminated:"):
		b.Broker.Publish(&events.Event{Kind: events.TWXSCRIPTTERM})
	case strings.HasPrefix(clean, "*** WARNING *** No locating beam found for sector"):
		b.Broker.Publish(&events.Event{Kind: events.BLINDJUMP})
	case strings.HasPrefix(clean, "Locating beam pinpointed, TransWarp Locked."):
		b.Broker.Publish(&events.Event{Kind: events.TWARPLOCKED})
	case strings.HasPrefix(clean, "You do not have enough Fuel Ore to make the jump."):
		b.Broker.Publish(&events.Event{Kind: events.TWARPLOWFUEL})
	case strings.HasPrefix(clean, "How many Atomic Detonators do you want"):
		b.parsers[parsers.BUYDETONATORS] = parsers.NewParseBuyDetonators(b.Broker)
	case strings.HasPrefix(clean, "How many Genesis Torpedoes do you want"):
		b.parsers[parsers.BUYDETONATORS] = parsers.NewParseBuyGTorp(b.Broker)
	case strings.Contains(clean, "Planet is now in sector"):
		b.Broker.Publish(&events.Event{Kind: events.PLANETWARPCOMPLETE})
	case strings.HasPrefix(clean, "The port Guards surround you"):
		b.Broker.Publish(&events.Event{Kind: events.BUSTED, DataInt: b.data.Status.Sector})
	case strings.Contains(clean, "has warps to sector(s) :"):
		b.parsers[parsers.SECTORWARPS] = parsers.NewSectorWarpsParser(b.Broker, b.data)
	case strings.Contains(clean, "empty cargo holds."):
		parts := tradeComplete.FindStringSubmatch(clean)
		if len(parts) == 2 {
			empty, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Printf("failed to parse empty holds: %s\n", err.Error())
				return
			}
			b.Broker.Publish(&events.Event{
				Kind:    events.TRADECOMPLETE,
				DataInt: empty,
			})
		}
	case strings.Contains(clean, "We're not interested."):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "When you want to make me a real offer, drop back by."):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "Swine, go peddle your wares somewhere else, you make me sick."):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "I see you are as stupid as you look, get lost..."):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "HA!  You think me a fool?  Thats insane!  Get out of here!"):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "Get lost creep, that junk isn't worth half that much!"):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "I think you'd better leave if you value your life!"):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "How have you survived this long?  Get lost, I'm not interested."):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOTINTERESTED})
	case strings.Contains(clean, "Available Ship Scan"):
		b.parsers[parsers.AVAILABLESHIPS] = parsers.NewParseAvailableShipScan(b.Broker, b.data)
	case strings.HasPrefix(clean, "You have never visited sector"):
		b.Broker.Publish(&events.Event{Kind: events.NOTVISITEDSECTORMSG})
	case strings.HasPrefix(clean, " Items     Status  Trading On Dock OnBoard"):
		b.parsers[parsers.PORTEQUTOSTEAL] = parsers.NewParseSteal(b.Broker)
	case strings.HasPrefix(clean, "For stealing from this port, your alignment went down"):
		b.Broker.Publish(&events.Event{Kind: events.STEALRESULT, ID: string(events.CRIMESUCCESS)})
	case strings.HasPrefix(clean, "A  Cargo holds     :"):
		parts := holdsToBuy.FindStringSubmatch(clean)
		if len(parts) == 2 {
			holds, err := strconv.Atoi(parts[1])
			if err != nil {
				return
			}
			b.Broker.Publish(&events.Event{
				Kind:    events.HOLDSTOBUY,
				DataInt: holds,
			})
		}
	case strings.HasPrefix(clean, "B  Fighters        :"):
		parts := figsToBuy.FindStringSubmatch(clean)
		if len(parts) == 2 {
			figs, err := strconv.Atoi(parts[1])
			if err != nil {
				return
			}
			b.Broker.Publish(&events.Event{
				Kind:    events.FIGSTOBUY,
				DataInt: figs,
			})
		}
	case strings.HasPrefix(clean, "C  Shield Points   :"):
		parts := shieldsToBuy.FindStringSubmatch(clean)
		if len(parts) == 2 {
			shields, err := strconv.Atoi(parts[1])
			if err != nil {
				return
			}
			b.Broker.Publish(&events.Event{
				Kind:    events.SHIELDSTOBUY,
				DataInt: shields,
			})
		}
	case strings.HasPrefix(clean, "That is not an available ship."):
		b.Broker.Publish(&events.Event{Kind: events.SHIPNOTAVAILABLE})
	case strings.HasPrefix(clean, "I have no information about a port in that sector."):
		b.Broker.Publish(&events.Event{Kind: events.PORTNOINFO})
	case strings.HasPrefix(clean, "Trade Wars 2002 Game Configuration and Status"):
		b.parsers[parsers.CONFIGDISPLAY] = parsers.NewParseConfig(b.Broker, b.data)
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
		e.DataInt = sector
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
	case "Stop in this":
		e.ID = events.STOPINSECTORPROMPT
	case "Mined Sector":
		e.ID = events.MINEDSECTORPROMPT
	case "How many hol":
		parts := promptBuySell.FindStringSubmatch(clean)
		if len(parts) != 3 {
			break
		}
		switch parts[2] {
		case "buy":
			e.ID = events.BUYPROMPT
		case "sell":
			e.ID = events.SELLPROMPT
		default:
			fmt.Printf("unknown buy/sell prompt word: %s\n", parts[2])
			return
		}

		switch parts[1] {
		case "Fuel Ore":
			e.Data = string(models.FUEL)
		case "Organics":
			e.Data = string(models.ORG)
		case "Equipment":
			e.Data = string(models.EQU)
		default:
			fmt.Printf("unknown buy/sell prompt product: %s\n", parts[1])
			return
		}

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

func parsePCreateArgs(args string) (map[string]int, error) {
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("must have an even number of args. Ex: 2L1O2H")
	}

	ret := map[string]int{}
	var count int
	var err error

	for i, c := range args {
		if i%2 == 0 {
			count, err = strconv.Atoi(string(c))
			if err != nil {
				return nil, err
			}
		} else {
			ret[string(c)] = count
		}
	}
	return ret, nil
}

func parsePStripArgs(args string) (int, int, error) {
	parts := strings.Split(args, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("got %d args; need exactly 2", len(parts))
	}

	fromID, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	toID, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}

	return fromID, toID, nil
}
