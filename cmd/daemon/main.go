package main

import (
	"context"
	"fmt"
	"focus/internal/events"
	"focus/internal/state"
	"focus/internal/sys"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const socketPath = "/tmp/focus.sock"
const (
	idleThresholdSeconds = 300
	idlePollSeconds      = 1
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServer(state.Get(), sys.RealActions{})

	// Cleanup socket on exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
		os.Remove(socketPath)
		os.Exit(0)
	}()

	listener, err := events.Start(ctx, idleThresholdSeconds, idlePollSeconds)
	if err != nil {
		log.Printf("focus-events startup failed: %v", err)
	} else {
		go consumeHelperEvents(listener.Events)
		go logHelperErrors(listener.Errors)
	}

	// Remove old socket if it exists
	os.Remove(socketPath)

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Printf("Listen error: %v\n", err)
		return
	}
	defer l.Close()

	fmt.Println("Go Daemon listening on", socketPath)
	go state.Get().StartIdleMonitor()

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}

		go srv.HandleConnection(conn)
	}
}

func consumeHelperEvents(eventCh <-chan events.Event) {
	for event := range eventCh {
		switch event.Kind {
		case events.KindScreen:
			switch event.State {
			case "locked":
				state.Get().SetSystemLocked(true)
			case "unlocked":
				state.Get().SetSystemLocked(false)
			}
		}
		log.Printf("focus-events event=%s state=%s fields=%v", event.Kind, event.State, event.Fields)
	}
}

func logHelperErrors(errCh <-chan error) {
	for err := range errCh {
		if err != nil {
			log.Printf("focus-events error: %v", err)
		}
	}
}
