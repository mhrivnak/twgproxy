package main

import (
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

	user, err := s.Accept()
	if err != nil {
		panic(err)
	}

	var stop chan interface{}

	gameParseR, gameParseW := io.Pipe()
	gameInputTee := io.TeeReader(game, gameParseW)
	go copyNotify(user, gameInputTee, stop)

	bot := bot.New(gameParseR, user, game)
	bot.Start(stop)

	<-stop
}

func copyNotify(dst io.Writer, src io.Reader, done chan interface{}) {
	io.Copy(dst, src)
	done <- struct{}{}
}
