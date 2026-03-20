package sys

import (
	"fmt"
	"os/exec"
)

type Actions interface {
	LockScreen()
	UnlockScreen()
	PlaySound(path string)
	Notify(title, message string)
}

type RealActions struct{}

type NoopActions struct{}

func realLockScreen() {
	cmd := exec.Command("xdg-screensaver", "lock")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error locking screen: %v\n", err)
	}
}

func realUnlockScreen() {
	cmd := exec.Command("cinnamon-screensaver-command", "-d")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error unlocking screen: %v\n", err)
	}
}

func realPlaySound(path string) {
	cmd := exec.Command("paplay", path)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error playing sound: %v\n", err)
	}
}

func realNotify(title, message string) {
	cmd := exec.Command("notify-send", "-i", "dialog-information", title, message)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error sending notification: %v\n", err)
	}
}

func (RealActions) LockScreen() {
	realLockScreen()
}

func (RealActions) UnlockScreen() {
	realUnlockScreen()
}

func (RealActions) PlaySound(path string) {
	realPlaySound(path)
}

func (RealActions) Notify(title, message string) {
	realNotify(title, message)
}

func (NoopActions) LockScreen()           {}
func (NoopActions) UnlockScreen()         {}
func (NoopActions) PlaySound(string)      {}
func (NoopActions) Notify(string, string) {}
