package defines

import (
	"context"

	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
)

type Control interface {
	Client() *client.Client
	GetConfig() ControlConfig
	SetConfig(ControlConfig)
	EnterRentalServer() error
	LeaveRentalServer() error
	GameInterface() *game_interface.GameInterface
	Resources() *resources_control.Resources
	TaskManager() TaskManager
	DeleteTask(Task)
	RunAllTask(context.Context, bool) error
	GetTaskID() int
	AddTask(Task)
}
