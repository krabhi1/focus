package main

import (
	"os"
	"strings"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
)

var (
	stdoutIsTerminalFn = defaultStdoutIsTerminal
	noColorDisabledFn  = defaultNoColorDisabled
)

func defaultStdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func defaultNoColorDisabled() bool {
	return strings.TrimSpace(os.Getenv("NO_COLOR")) != ""
}

func colorEnabled() bool {
	return stdoutIsTerminalFn() && !noColorDisabledFn()
}

func colorText(text, code string) string {
	if text == "" || !colorEnabled() {
		return text
	}
	return code + text + ansiReset
}

func colorTitle(text string) string   { return colorText(text, ansiBold+ansiCyan) }
func colorHeading(text string) string { return colorText(text, ansiCyan) }
func colorLabel(text string) string   { return colorText(text, ansiDim) }
func colorInfo(text string) string    { return colorText(text, ansiCyan) }
func colorSuccess(text string) string { return colorText(text, ansiGreen) }
func colorWarn(text string) string    { return colorText(text, ansiYellow) }
func colorError(text string) string   { return colorText(text, ansiRed) }
func colorMuted(text string) string   { return colorText(text, ansiDim) }
func colorPrompt(text string) string  { return colorText(text, ansiBold+ansiCyan) }

func colorStatusMessage(message string) string {
	switch {
	case message == "Idle":
		return colorMuted(message)
	case strings.HasPrefix(message, "Cooldown starting"):
		return colorWarn(message)
	case strings.HasPrefix(message, "Cooldown active"):
		return colorWarn(message)
	case strings.HasPrefix(message, "Task: ") && strings.Contains(message, "Status: break"):
		return colorWarn(message)
	case strings.HasPrefix(message, "Task: "):
		return colorSuccess(message)
	case strings.Contains(message, "No task history"):
		return colorMuted(message)
	default:
		return message
	}
}

func colorHistoryMessage(message string) string {
	if !colorEnabled() || message == "" {
		return message
	}

	lines := strings.Split(message, "\n")
	for i, line := range lines {
		parts := strings.Split(line, " | ")
		if len(parts) != 4 {
			continue
		}

		parts[0] = colorInfo(parts[0])
		parts[1] = colorSuccess(parts[1])
		parts[2] = colorInfo(parts[2])
		parts[3] = colorMuted(parts[3])
		lines[i] = strings.Join(parts, " | ")
	}
	return strings.Join(lines, "\n")
}
