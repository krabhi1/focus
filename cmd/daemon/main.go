package main

import (
	"encoding/gob"
	"fmt"
	"go-basic/internal/protocol"
	"go-basic/internal/state"
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

func main() {
	// Cleanup socket on exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Remove(socketPath)
		os.Exit(0)
	}()

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
	switch req.Command {
	case "start":
		res := handleStart(req.Payload.(protocol.StartRequest))
		err = gob.NewEncoder(conn).Encode(res)
		if err != nil {
			fmt.Printf("Encode error: %v\n", err)
		}
	case "status":
		res := handleStatus()
		err = gob.NewEncoder(conn).Encode(res)
		if err != nil {
			fmt.Printf("Encode error: %v\n", err)
		}
	case "stop":
		res := handleStop()
		err = gob.NewEncoder(conn).Encode(res)
		if err != nil {
			fmt.Printf("Encode error: %v\n", err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", req.Command)
	}

	//send response
	res := protocol.Response{
		Type:    "success",
		Payload: fmt.Sprintf("Received command: %s", req.Command),
	}
	err = gob.NewEncoder(conn).Encode(res)
	if err != nil {
		fmt.Printf("Encode error: %v\n", err)
		return
	}

}
func handleStart(req protocol.StartRequest) protocol.Response {
	if state.Global.CurrentTask != nil {
		fmt.Printf("A task is already running: %+v\n", state.Global.CurrentTask)
		return protocol.Response{
			Type: "error",
			Payload: protocol.ErrorResponse{
				Message: fmt.Sprintf("A task is already running: %s", state.Global.CurrentTask.Title),
			},
		}
	}
	task := state.Global.NewTask(req.Title, req.Duration)
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

func handleStop() protocol.Response {
	if state.Global.CurrentTask == nil {
		return protocol.Response{
			Type: "success",
			Payload: protocol.SuccessResponse{
				Message: "No active task to stop",
			},
		}
	}
	defer state.Global.StopCurrentTask()
	return protocol.Response{
		Type: "success",
		Payload: protocol.SuccessResponse{
			Message: fmt.Sprintf("Stopped the task: %s", state.Global.CurrentTask.Title),
		},
	}
}
