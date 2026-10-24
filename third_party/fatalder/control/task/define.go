package task

import (
	"errors"

	"github.com/Yeah114/Fatalder/defines"
)

type TaskType string

const (
	TaskTypeCommand = "command"
	TaskTypeSleep = "sleep"
	TaskTypeChat = "chat"
	TaskTypeMove = "move"
	TaskTypeBuild = "build"
	TaskTypeExport = "export"
)


var TaskTypes = map[TaskType]func() defines.Task {
	TaskTypeCommand: func() defines.Task { return NewCommandTask() },
	TaskTypeSleep: func() defines.Task { return NewSleepTask() },
	TaskTypeChat: func() defines.Task { return NewChatTask() },
	TaskTypeMove: func() defines.Task { return NewMoveTask() },
	TaskTypeBuild: func() defines.Task { return NewBuildTask() },
	TaskTypeExport: func() defines.Task { return NewExportTask() },
}

func (t TaskType) GetTask() defines.Task {
	task := TaskTypes[t]
	if task == nil {
		return nil
	}
	return task()
}

var ErrInvalidInterruptData = errors.New("invalid interrupt data")
var ErrInvalidStructureFile = errors.New("invalid structure file")
var ErrInvalidControl = errors.New("invalid control")
