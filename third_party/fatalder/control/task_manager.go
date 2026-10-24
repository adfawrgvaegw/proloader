package control

import "github.com/Yeah114/Fatalder/defines"

type TaskManager struct {
	tasks map[int]defines.Task
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[int]defines.Task),
	}
}

func (t *TaskManager) AddTask(task defines.Task) {
	t.tasks[task.ID()] = task
}

func (t *TaskManager) RemoveTask(id int) {
	delete(t.tasks, id)
}

func (t *TaskManager) GetTask(id int) defines.Task {
	return t.tasks[id]
}

func (t *TaskManager) GetTasks() map[int]defines.Task {
	return t.tasks
}
