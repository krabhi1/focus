package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"focus/internal/protocol"
	"net"
	"os"
	"time"
)

type DurationArg time.Duration // any of [short(15m),medium(30m),long(60m),deep(90min)]
func (d *DurationArg) Set(value string) error {
	// Mapping inside the function to keep it clean
	durations := map[string]time.Duration{
		"short":  15 * time.Minute,
		"medium": 30 * time.Minute,
		"long":   60 * time.Minute,
		"deep":   90 * time.Minute,
	}

	if val, ok := durations[value]; ok {
		*d = DurationArg(val)
		return nil
	}
	return fmt.Errorf("choose [short(15m),medium(30m),long(60m),deep(90min)")
}

func (d *DurationArg) String() string {
	return time.Duration(*d).String()
}

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

		var durationArg DurationArg
		startCmd.Var(&durationArg, "duration", "Duration [short, medium, long, deep]")
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
		duration := time.Duration(durationArg)

		req := protocol.Request{
			Command: "start",
			Payload: protocol.StartRequest{
				Title:    *name,
				Duration: duration,
			},
		}
		fmt.Println(*name, duration, req)
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		fmt.Println("Response:", res)
	case "cancel":
		req := protocol.Request{
			Command: "cancel",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		fmt.Println("Response:", res)
	case "status":
		req := protocol.Request{
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
