package main

import (
	"encoding/gob"
	"fmt"
	"go-basic/pkg/protocol"
	"net"
	"os"
	"os/signal"
	"syscall"
)

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
	fmt.Printf("Received  %+v\n", req)

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
