package task_group_manager

import (
	"github.com/google/uuid"
	"github.com/Yeah114/Fatalder/defines"
)

type RunningTaskGroup struct {
	Name string
	UUID uuid.UUID
	Control defines.Control
	Tasks []defines.Task
}
