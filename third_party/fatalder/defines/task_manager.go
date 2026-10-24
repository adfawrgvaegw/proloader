package defines

type TaskManager interface {
	AddTask(task Task)
	RemoveTask(id int)
	GetTask(id int) Task
	GetTasks() map[int]Task
}
