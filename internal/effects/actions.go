package effects

type Actions interface {
	LockScreen()
	UnlockScreen()
	Sleep()
	PlaySound(path string)
	Notify(title, message string)
}
