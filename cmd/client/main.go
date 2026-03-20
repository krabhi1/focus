package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"focus/internal/state"
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
	return fmt.Errorf("choose one of [short(15m), medium(30m), long(60m), deep(90m)]")
}

func (d *DurationArg) String() string {
	return time.Duration(*d).String()
}

func main() {
	command := "status"
	if len(os.Args) >= 2 {
		command = os.Args[1]
	}

	switch command {
	case "help", "-h", "--help":
		printHelp()
		return
	case "version":
		printVersion()
		return
	case "uninstall":
		uninstallCmd := flag.NewFlagSet("uninstall", flag.ExitOnError)
		prefix := uninstallCmd.String("prefix", "", "Install prefix (defaults to the current binary directory)")
		uninstallCmd.Parse(os.Args[2:])

		if err := Uninstall(*prefix); err != nil {
			fmt.Println("Uninstall failed:", err)
			return
		}
		fmt.Println("Focus uninstalled.")
		return
	case "update":
		updateCmd := flag.NewFlagSet("update", flag.ExitOnError)
		targetVersion := updateCmd.String("version", "", "Release version to install (default: latest)")
		prefix := updateCmd.String("prefix", "", "Install prefix (defaults to the current binary directory)")
		yes := updateCmd.Bool("yes", false, "Skip confirmation prompt")
		updateCmd.Parse(os.Args[2:])

		if err := Update(*targetVersion, *prefix, *yes); err != nil {
			fmt.Println("Update failed:", err)
			return
		}
		fmt.Println("Focus updated.")
		return
	case "start":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println("Daemon not running.")
			return
		}
		defer conn.Close()

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
			Start: &protocol.StartRequest{
				Title:    *name,
				Duration: duration,
			},
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		printResponse(res)
	case "cancel":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println("Daemon not running.")
			return
		}
		defer conn.Close()

		req := protocol.Request{
			Command: "cancel",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		printResponse(res)
	case "history":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println("Daemon not running.")
			return
		}
		defer conn.Close()

		req := protocol.Request{
			Command: "history",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		printResponse(res)
	case "status":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println("Daemon not running.")
			return
		}
		defer conn.Close()

		req := protocol.Request{
			Command: "status",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			return
		}
		printResponse(res)
	default:
		fmt.Printf("Invalid command: %s\n\n", command)
		printHelp()
	}
}

func printHelp() {
	fmt.Println("Focus CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  focus status")
	fmt.Println("  focus start --name <task> --duration <short|medium|long|deep>")
	fmt.Println("  focus cancel")
	fmt.Println("  focus history")
	fmt.Println("  focus version")
	fmt.Println("  focus update [--version <tag>] [--prefix <path>] [--yes]")
	fmt.Println("  focus uninstall [--prefix <path>]")
	fmt.Println("  focus help")
}

func connectDaemon() (net.Conn, error) {
	return net.Dial("unix", state.SocketPath())
}

func SendRequest(conn net.Conn, req protocol.Request) (protocol.Response, error) {
	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		return protocol.Response{}, err
	}

	var res protocol.Response
	err := gob.NewDecoder(conn).Decode(&res)
	if err != nil {
		return protocol.Response{}, err
	}
	return res, err
}

func printResponse(res protocol.Response) {
	switch {
	case res.Success != nil:
		fmt.Println(res.Success.Message)
	case res.Error != nil:
		fmt.Println(res.Error.Message)
	default:
		fmt.Printf("%s: empty response\n", res.Type)
	}
}
