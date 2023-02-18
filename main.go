package main

import (
	"fmt"
	"io"
	"net"

	"github.com/mhrivnak/twgproxy/pkg/bot"
)

func main() {
	game, err := net.Dial("tcp", "localhost:2300")
	if err != nil {
		panic(err)
	}
	defer game.Close()

	s, err := net.Listen("tcp", ":5555")
	if err != nil {
		panic(err)
	}
	defer s.Close()

	gameParseR, gameParseW := io.Pipe()
	bot := bot.New(gameParseR, game)

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
