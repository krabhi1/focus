package effects

type Actions interface {
	LockScreen()
	UnlockScreen()
	PlaySound(path string)
	Notify(title, message string)
}
