package defines

import "context"

type Task interface {
	SetControl(Control)
	ID() int
	Type() string
	FromJSON(string) error
	ToJSON() (string, error)
	Start(context.Context) error
	Pause() error
	Resume() error
	Stop() error
	IsRunning() bool
	IsPaused() bool
	MakeInterruptJSON() (string, error)
	RestoreInterruptJSON(string) error
}
