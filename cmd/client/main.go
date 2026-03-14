package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"go-basic/pkg/protocol"
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("unix", "/tmp/focus.sock")
	if err != nil {
		fmt.Println("Daemon not running.")
		return
	}
	defer conn.Close()

	// conn.Write([]byte(os.Args[1]))

	// buf := make([]byte, 512)
	// n, _ := conn.Read(buf)
	// fmt.Println(string(buf[:n]))
	if len(os.Args) < 2 {
		fmt.Println("TODO status")
		return
	}
	switch os.Args[1] {
	case "start":
		startCmd := flag.NewFlagSet("start", flag.ExitOnError)
		name := startCmd.String("name", "", "Task name")
		duration := startCmd.Duration("duration", 0, "Duration of the task ex 20s,15m")
		startCmd.Parse(os.Args[2:])
		required := map[string]bool{"name": false, "duration": false}
		startCmd.Visit(func(f *flag.Flag) {
			required[f.Name] = true
		})
		for _, val := range required {
			if !val {
				startCmd.Usage()
				return
			}
		}
		req := protocol.Request{
			Command: "start",
			Payload: protocol.StartRequest{
				Title:    *name,
				Duration: *duration,
			},
		}
		fmt.Println(*name, *duration, req)
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		fmt.Println("Response:", res)
	case "stop":
	case "status":
		req:= protocol.Request{
			Command: "status",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		fmt.Println("Response:", res)
	default:
		fmt.Println("Invalid command")
	}
}

func SendRequest(conn net.Conn, req protocol.Request) (protocol.Response, error) {
	//Send the request
	gob.NewEncoder(conn).Encode(req)
	//Wait for the response
	var res protocol.Response
	err := gob.NewDecoder(conn).Decode(&res)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return protocol.Response{}, err
	}
	return res, err
}
