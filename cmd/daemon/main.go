package main

import (
	"encoding/gob"
	"fmt"
	"go-basic/internal/protocol"
	"go-basic/internal/sys"
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

var CurrentTask *Task = nil

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
	if CurrentTask != nil {
		fmt.Printf("A task is already running: %+v\n", CurrentTask)
		return protocol.Response{
			Type: "error",
			Payload: protocol.ErrorResponse{
				Message: fmt.Sprintf("A task is already running: %s", CurrentTask.Title),
			},
		}
	}
	CurrentTask = &Task{
		Title:      req.Title,
		Duration:   req.Duration,
		StartTime:  time.Now(),
		ExpireTime: time.Now().Add(req.Duration),
	}
	//create a timer before 10 seconds of expire time to notify user
	beforeExpire := time.Until(CurrentTask.ExpireTime.Add(-10 * time.Second))
	if beforeExpire > 0 {
		time.AfterFunc(beforeExpire, func() {
			sys.Notify("Task expiring soon", fmt.Sprintf("'%s' will expire in 10 seconds", CurrentTask.Title))
			time.Sleep(2 * time.Second)
			sys.PlaySound("assets/task-ending.mp3")
		})
	}
	time.AfterFunc(req.Duration, func() {
		fmt.Printf("Task expired: %+v\n", CurrentTask)
		sys.Notify("Task expired", fmt.Sprintf("'%s' has expired. Screen is going to Lock", CurrentTask.Title))
		time.Sleep(5 * time.Second) // Give user some time to see the notification before locking
		sys.LockScreen()
		CurrentTask = nil
	})

	fmt.Printf("Started task: %+v\n", CurrentTask)
	return protocol.Response{
		Type: "success",
		Payload: protocol.SuccessResponse{
			Message: fmt.Sprintf("Started task: %s for %s", req.Title, req.Duration),
		},
	}
}

func handleStatus() protocol.Response {
	if CurrentTask == nil {
		return protocol.Response{
			Type: "success",
			Payload: protocol.SuccessResponse{
				Message: "No active task",
			},
		}
	}
	return protocol.Response{
		Type: "success",
		Payload: protocol.SuccessResponse{
			Message: fmt.Sprintf("Current task: %s, expires in %s", CurrentTask.Title, time.Until(CurrentTask.ExpireTime)),
		},
	}
}
