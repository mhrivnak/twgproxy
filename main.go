package main

import (
	"fmt"
	"io"
	"net"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mhrivnak/twgproxy/pkg/bot"
	"github.com/mhrivnak/twgproxy/pkg/models/persist"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("must provide path to db file as the only argument")
		os.Exit(1)
	}

	game, err := net.Dial("tcp", "localhost:2300")
	if err != nil {
		panic(err)
	}
	defer game.Close()

	db, err := gorm.Open(sqlite.Open(os.Args[1]), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	db.AutoMigrate(&persist.Sector{})
	db.AutoMigrate(&persist.Warp{})

	s, err := net.Listen("tcp", ":5555")
	if err != nil {
		panic(err)
	}
	defer s.Close()

	gameParseR, gameParseW := io.Pipe()
	bot := bot.New(gameParseR, game, db)

	for {
		user, err := s.Accept()
		if err != nil {
			panic(err)
		}

		copyStop := make(chan interface{})
		gameInputTee := io.TeeReader(game, gameParseW)
		go copyNotify(user, gameInputTee, copyStop)

		botStop := make(chan interface{})
		bot.Start(user, botStop)

		select {
		case <-copyStop:
			fmt.Println("got STOP signal from copyNotify")
		case <-botStop:
			fmt.Println("got STOP signal from bot")
		}
	}
}

func copyNotify(dst io.Writer, src io.Reader, done chan interface{}) {
	io.Copy(dst, src)
	close(done)
}
