package main

import (
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"focus/internal/protocol"
	"focus/internal/storage"
	"io"
	"net"
	"os"
	"strings"
)

type DurationArg string

func (d *DurationArg) Set(value string) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "short", "medium", "long", "deep":
		*d = DurationArg(normalized)
		return nil
	default:
		return fmt.Errorf("choose one of [short, medium, long, deep]")
	}
}

func (d *DurationArg) String() string {
	return string(*d)
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
	case "doctor":
		doctorCmd := flag.NewFlagSet("doctor", flag.ExitOnError)
		doctorCmd.SetOutput(io.Discard)
		all := doctorCmd.Bool("all", false, "Show full runtime debug output")
		doctorCmd.Parse(os.Args[2:])
		runDoctor(*all)
		return
	case "config":
		if err := runConfig(os.Args[2:], reloadDaemon); err != nil {
			fmt.Println(colorError("Config failed:"), err)
			return
		}
		return
	case "uninstall":
		uninstallCmd := flag.NewFlagSet("uninstall", flag.ExitOnError)
		prefix := uninstallCmd.String("prefix", "", "Install prefix (defaults to the current binary directory)")
		uninstallCmd.Parse(os.Args[2:])

		if err := Uninstall(*prefix); err != nil {
			fmt.Println(colorError("Uninstall failed:"), err)
			return
		}
		fmt.Println(colorSuccess("Focus uninstalled."))
		return
	case "update":
		updateCmd := flag.NewFlagSet("update", flag.ExitOnError)
		targetVersion := updateCmd.String("version", "", "Release version to install (default: latest)")
		prefix := updateCmd.String("prefix", "", "Install prefix (defaults to the current binary directory)")
		yes := updateCmd.Bool("yes", false, "Skip confirmation prompt")
		updateCmd.Parse(os.Args[2:])

		if err := Update(*targetVersion, *prefix, *yes); err != nil {
			fmt.Println(colorError("Update failed:"), err)
			return
		}
		fmt.Println(colorSuccess("Focus updated."))
		return
	case "start":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println(colorError("Daemon not running."))
			return
		}
		defer conn.Close()

		req, err := buildStartRequest(os.Args[2:])
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Println(colorError("Error:"), err)
			return
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println(colorError("Error sending request:"), err)
			return
		}
		printResponse(res)
	case "cancel":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println(colorError("Daemon not running."))
			return
		}
		defer conn.Close()

		req := protocol.Request{
			Command: "cancel",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println(colorError("Error sending request:"), err)
			return
		}
		printResponse(res)
	case "history":
		req, err := buildHistoryRequest(os.Args[2:])
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Println(colorError("Error:"), err)
			return
		}

		conn, err := connectDaemon()
		if err != nil {
			fmt.Println(colorError("Daemon not running."))
			return
		}
		defer conn.Close()

		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println(colorError("Error sending request:"), err)
			return
		}
		printResponse(res)
	case "reload":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println(colorError("Daemon not running."))
			return
		}
		defer conn.Close()

		req := protocol.Request{
			Command: "reload",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println(colorError("Error sending request:"), err)
			return
		}
		printResponse(res)
	case "status":
		conn, err := connectDaemon()
		if err != nil {
			fmt.Println(colorError("Daemon not running."))
			return
		}
		defer conn.Close()

		req := protocol.Request{
			Command: "status",
		}
		res, err := SendRequest(conn, req)
		if err != nil {
			fmt.Println(colorError("Error sending request:"), err)
			return
		}
		printResponse(res)
	default:
		fmt.Printf("%s %s\n\n", colorError("Invalid command:"), colorInfo(command))
		printHelp()
	}
}

func printHelp() {
	fmt.Println(colorTitle("Focus CLI"))
	fmt.Println("")
	fmt.Println(colorHeading("Usage:"))
	fmt.Println("  " + colorInfo("focus status"))
	fmt.Println("  " + colorInfo("focus start --name <task> --duration <short|medium|long|deep> [--no-break]"))
	fmt.Println("  " + colorInfo("focus cancel"))
	fmt.Println("  " + colorInfo("focus history [--all]"))
	fmt.Println("  " + colorInfo("focus reload"))
	fmt.Println("  " + colorInfo("focus config <key> <value>"))
	fmt.Println("  " + colorInfo("focus doctor [--all]"))
	fmt.Println("  " + colorInfo("focus version"))
	fmt.Println("  " + colorInfo("focus update [--version <tag>] [--prefix <path>] [--yes]"))
	fmt.Println("  " + colorInfo("focus uninstall [--prefix <path>]"))
	fmt.Println("  " + colorInfo("focus help"))
}

func printHistoryHelp() {
	fmt.Println(colorHeading("Usage:"))
	fmt.Println("  " + colorInfo("focus history [--all]"))
	fmt.Println("")
	fmt.Println(colorHeading("Flags:"))
	fmt.Println("  " + colorLabel("--all") + "   Show all persisted history instead of today-only history")
}

func buildHistoryRequest(args []string) (protocol.Request, error) {
	historyCmd := flag.NewFlagSet("history", flag.ContinueOnError)
	historyCmd.SetOutput(io.Discard)
	all := historyCmd.Bool("all", false, "Show all persisted history instead of today-only history")
	historyCmd.Usage = func() {
		printHistoryHelp()
	}
	if err := historyCmd.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			historyCmd.Usage()
		}
		return protocol.Request{}, err
	}
	return protocol.Request{
		Command:    "history",
		HistoryAll: *all,
	}, nil
}

func buildStartRequest(args []string) (protocol.Request, error) {
	startCmd := flag.NewFlagSet("start", flag.ContinueOnError)
	startCmd.SetOutput(io.Discard)
	startCmd.Usage = func() {
		printStartHelp()
	}

	name := startCmd.String("name", "", "Task name")
	noBreak := startCmd.Bool("no-break", false, "Skip the in-task break for this task")

	var durationArg DurationArg
	startCmd.Var(&durationArg, "duration", "Duration [short, medium, long, deep]")
	if err := startCmd.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			startCmd.Usage()
		}
		return protocol.Request{}, err
	}
	required := map[string]bool{"name": false, "duration": false}
	startCmd.Visit(func(f *flag.Flag) {
		required[f.Name] = true
	})
	for _, val := range required {
		if !val {
			printStartHelp()
			return protocol.Request{}, flag.ErrHelp
		}
	}
	return protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:   *name,
			Preset:  string(durationArg),
			NoBreak: *noBreak,
		},
	}, nil
}

func printStartHelp() {
	fmt.Println(colorHeading("Usage:"))
	fmt.Println("  " + colorInfo("focus start --name <task> --duration <short|medium|long|deep> [--no-break]"))
	fmt.Println("")
	fmt.Println(colorHeading("Flags:"))
	fmt.Println("  " + colorLabel("--name") + "       Task name")
	fmt.Println("  " + colorLabel("--duration") + "   Duration [short, medium, long, deep]")
	fmt.Println("  " + colorLabel("--no-break") + "   Skip the in-task break for this task")
}

func connectDaemon() (net.Conn, error) {
	return net.Dial("unix", storage.DefaultSocketPath())
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
		message := res.Success.Message
		if strings.Contains(message, "\n") && strings.Contains(message, "completed | started ") {
			fmt.Println(colorHistoryMessage(message))
			return
		}
		fmt.Println(colorStatusMessage(message))
	case res.Error != nil:
		fmt.Println(colorError(res.Error.Message))
	default:
		fmt.Printf("%s: %s\n", colorError(res.Type), colorMuted("empty response"))
	}
}
