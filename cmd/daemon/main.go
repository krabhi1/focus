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
	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()
	srv := NewServer(state.Get(), sys.RealActions{})

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
	defer func() {
		_ = l.Close()
		_ = os.Remove(socketPath)
	}()

	fmt.Println("Go Daemon listening on", socketPath)
	go state.Get().StartIdleMonitor(ctx)

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
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
				state.Get().OnScreenLocked()
			case "unlocked":
				state.Get().SetSystemLocked(false)
				state.Get().OnScreenUnlocked()
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
