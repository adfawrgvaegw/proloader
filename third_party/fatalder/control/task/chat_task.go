package task

import (
	"context"
	"encoding/json"

	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/defines"
)

type ChatTask struct {
	control  control.Control
	id       int
	TaskType TaskType `json:"type"`
	Content  string   `json:"content"`
}

func NewChatTask() *ChatTask {
	return &ChatTask{
		TaskType: TaskTypeChat,
	}
}


func (c *ChatTask) SetControl(ctrl defines.Control) {
	ctrlTyped, ok := ctrl.(*control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	c.control = *ctrlTyped
	c.id = c.control.GetTaskID()
}

func (c *ChatTask) ID() int {
	return c.id
}

func (c *ChatTask) Type() string {
	return string(c.TaskType)
}

func (c *ChatTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), c)
}

func (c *ChatTask) ToJSON() (string, error) {
	bs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (c *ChatTask) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return c.control.GameInterface().Commands().SendChat(c.Content)
}

func (c *ChatTask) Pause() error {
	return nil
}

func (c *ChatTask) Resume() error {
	return nil
}

func (c *ChatTask) Stop() error {
	return nil
}

func (c *ChatTask) IsRunning() bool {
	return false
}

func (c *ChatTask) IsPaused() bool {
	return false
}

func (c *ChatTask) MakeInterruptJSON() (string, error) {
	return "", nil
}

func (c *ChatTask) RestoreInterruptJSON(string) error {
	return nil
}
