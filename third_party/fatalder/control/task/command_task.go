package task

import (
	"context"
	"encoding/json"

	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/defines"
)

type CommandTask struct {
	control  control.Control
	id       int
	TaskType TaskType `json:"type"`
	Command  string   `json:"command"`
	Identity string   `json:"identity"`
}

func NewCommandTask() *CommandTask {
	return &CommandTask{
		TaskType: TaskTypeCommand,
	}
}

func (c *CommandTask) SetControl(ctrl defines.Control) {
	ctrlTyped, ok := ctrl.(*control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	c.control = *ctrlTyped
	c.id = c.control.GetTaskID()
}

func (c *CommandTask) ID() int {
	return c.id
}

func (c *CommandTask) Type() string {
	return string(c.TaskType)
}

func (c *CommandTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), c)
}

func (c *CommandTask) ToJSON() (string, error) {
	bs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (c *CommandTask) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	commands := c.control.GameInterface().Commands()
	switch c.Identity {
	case "player":
		return commands.SendPlayerCommand(c.Command)
	case "ws":
		return commands.SendWSCommand(c.Command)
	case "settings":
		return commands.SendSettingsCommand(c.Command, false)
	default:
		return commands.SendPlayerCommand(c.Command)
	}
}

func (c *CommandTask) Pause() error {
	return nil
}

func (c *CommandTask) Resume() error {
	return nil
}

func (c *CommandTask) Stop() error {
	return nil
}

func (c *CommandTask) IsRunning() bool {
	return false
}

func (c *CommandTask) IsPaused() bool {
	return false
}

func (c *CommandTask) MakeInterruptJSON() (string, error) {
	return "", nil
}

func (c *CommandTask) RestoreInterruptJSON(string) error {
	return nil
}
