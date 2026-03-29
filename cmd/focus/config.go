package main

import (
	"fmt"
	"focus/internal/protocol"
	"focus/internal/storage"
	"strings"
)

func runConfig(args []string, reload func() error) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		printConfigHelp(storage.DefaultRuntimeConfig())
		return nil
	}
	if len(args) == 1 {
		return showConfigValue(args[0])
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: focus config <key> [<value>]")
	}

	key := strings.TrimSpace(args[0])
	value := strings.TrimSpace(args[1])
	if key == "" || value == "" {
		return fmt.Errorf("usage: focus config <key> [<value>]")
	}

	path, err := storage.DefaultPath()
	if err != nil {
		return err
	}
	if err := storage.UpdateConfigValue(path, key, value); err != nil {
		return err
	}

	if reload != nil {
		if err := reload(); err != nil {
			fmt.Printf("%s %v\n", colorWarn("Config updated. Daemon reload skipped:"), err)
			return nil
		}
		fmt.Println(colorSuccess("Config updated and daemon reloaded."))
		return nil
	}

	fmt.Println(colorSuccess("Config updated."))
	return nil
}

func showConfigValue(key string) error {
	path, err := storage.DefaultPath()
	if err != nil {
		return err
	}

	fileCfg, _, err := storage.Load(path)
	if err != nil {
		return err
	}

	resolved, err := storage.ResolveRuntimeConfig(storage.DefaultRuntimeConfig(), fileCfg, storage.Overrides{})
	if err != nil {
		return err
	}

	current, err := storage.DescribeConfigKey(resolved, key)
	if err != nil {
		return err
	}
	defaultValue, err := storage.DescribeConfigKey(storage.DefaultRuntimeConfig(), key)
	if err != nil {
		return err
	}

	if current == defaultValue {
		fmt.Printf("%s = %s (default)\n", colorInfo(key), colorSuccess(current))
		return nil
	}
	fmt.Printf("%s = %s (default: %s)\n", colorInfo(key), colorSuccess(current), colorMuted(defaultValue))
	return nil
}

func isHelpArg(arg string) bool {
	switch strings.TrimSpace(strings.ToLower(arg)) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func printConfigHelp(defaults storage.RuntimeConfig) {
	fmt.Println(colorHeading("Usage:"))
	fmt.Println("  " + colorInfo("focus config <key> [<value>]"))
	fmt.Println("")
	fmt.Println(colorHeading("Supported keys:"))
	for _, key := range storage.SupportedConfigKeys() {
		value, err := storage.DescribeConfigKey(defaults, key)
		if err != nil {
			continue
		}
		fmt.Printf("  %s (default: %s)\n", colorInfo(key), colorMuted(value))
	}
	fmt.Println("")
	fmt.Println(colorHeading("Examples:"))
	fmt.Println("  " + colorInfo("focus config idle.lock_after 3m"))
	fmt.Println("  " + colorInfo("focus config relock_delay 0s"))
	fmt.Println("  " + colorInfo("focus config alert.repeat_count 3"))
	fmt.Println("")
	fmt.Println(colorMuted("Use dot notation for nested keys. Use one argument to read a value."))
}

func reloadDaemon() error {
	conn, err := connectDaemon()
	if err != nil {
		return err
	}
	defer conn.Close()

	req := protocol.Request{Command: "reload"}
	res, err := SendRequest(conn, req)
	if err != nil {
		return err
	}
	if res.Error != nil {
		return fmt.Errorf("%s", res.Error.Message)
	}
	return nil
}
