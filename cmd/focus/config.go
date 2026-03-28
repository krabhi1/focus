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
			fmt.Printf("Config updated. Daemon reload skipped: %v\n", err)
			return nil
		}
		fmt.Println("Config updated and daemon reloaded.")
		return nil
	}

	fmt.Println("Config updated.")
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
		fmt.Printf("%s = %s (default)\n", key, current)
		return nil
	}
	fmt.Printf("%s = %s (default: %s)\n", key, current, defaultValue)
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
	fmt.Println("Usage:")
	fmt.Println("  focus config <key> [<value>]")
	fmt.Println("")
	fmt.Println("Supported keys:")
	for _, key := range storage.SupportedConfigKeys() {
		value, err := storage.DescribeConfigKey(defaults, key)
		if err != nil {
			continue
		}
		fmt.Printf("  %s (default: %s)\n", key, value)
	}
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  focus config idle.lock_after 3m")
	fmt.Println("  focus config relock_delay 0s")
	fmt.Println("  focus config cooldown_start_delay 2m")
	fmt.Println("")
	fmt.Println("Use dot notation for nested keys. Use one argument to read a value.")
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
