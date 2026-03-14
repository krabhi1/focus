package sys

import (
	"fmt"
	"os/exec"
)

func LockScreen() {
	cmd := exec.Command("xdg-screensaver", "lock")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error locking screen: %v\n", err)
	}
}

func PlaySound(path string) {
	cmd := exec.Command("paplay", path)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error playing sound: %v\n", err)
	}
}

func Notify(title, message string) {
	cmd := exec.Command("notify-send", "-i", "dialog-information", title, message)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error sending notification: %v\n", err)
	}
}
