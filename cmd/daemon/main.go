package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"focus/internal/events"
	"focus/internal/protocol"
	"focus/internal/state"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Task struct {
	Title      string
	Duration   time.Duration
	StartTime  time.Time
	ExpireTime time.Time
}

const socketPath = "/tmp/focus.sock"
const (
	idleThresholdSeconds = 300
	idlePollSeconds      = 1
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

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

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	var req protocol.Request
	err := gob.NewDecoder(conn).Decode(&req)
	if err != nil {
		fmt.Printf("Decode error: %v\n", err)
		return
	}
	fmt.Printf("Received ==> %+v\n", req)
	var res protocol.Response
	switch req.Command {
	case "start":
		res = handleStart(req.Payload.(protocol.StartRequest))
	case "status":
		res = handleStatus()
	case "cancel":
		res = handleCancel()
	default:
		fmt.Printf("Unknown command: %s\n", req.Command)
		res = protocol.Response{
			Type: "error",
			Payload: protocol.ErrorResponse{
				Message: fmt.Sprintf("Unknown command: %s", req.Command),
			},
		}
	}

	err = gob.NewEncoder(conn).Encode(res)
	if err != nil {
		fmt.Printf("Encode error: %v\n", err)
		return
	}

}

func consumeHelperEvents(eventCh <-chan events.Event) {
	for event := range eventCh {
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

func handleStart(req protocol.StartRequest) protocol.Response {
	if currentTitle, ok := state.Global.CurrentTaskTitle(); ok {
		fmt.Printf("A task is already running: %s\n", currentTitle)
		return protocol.Response{
			Type: "error",
			Payload: protocol.ErrorResponse{
				Message: fmt.Sprintf("A task is already running: %s", currentTitle),
			},
		}
	}
	task, _ := state.Global.NewTask(req.Title, req.Duration)
	return protocol.Response{
		Type: "success",
		Payload: protocol.SuccessResponse{
			Message: fmt.Sprintf("Started task: %s for %s", task.Title, task.Duration),
		},
	}
}

func handleStatus() protocol.Response {
	return protocol.Response{
		Type: "success",
		Payload: protocol.SuccessResponse{
			Message: state.Global.GetStatus(),
		},
	}
}

func handleCancel() protocol.Response {

	task, err := state.Global.CancelCurrentTask()
	if err != nil {
		return protocol.Response{
			Type: "error",
			Payload: protocol.ErrorResponse{
				Message: fmt.Sprintf("Failed to cancel task: %v", err),
			},
		}
	}
	return protocol.Response{
		Type: "success",
		Payload: protocol.SuccessResponse{
			Message: fmt.Sprintf("Cancelled the task: %s", task.Title),
		},
	}
}
