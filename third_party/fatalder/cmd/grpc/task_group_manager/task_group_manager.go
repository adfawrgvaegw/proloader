package task_group_manager

import (
	"context"
	"sync"

	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/google/uuid"
)

type TaskGroupManager struct {
	mu                sync.RWMutex
	runningTaskGroups map[uuid.UUID]*RunningTaskGroup
	order             []uuid.UUID
}

func NewTaskGroupManager() *TaskGroupManager {
	return &TaskGroupManager{
		runningTaskGroups: make(map[uuid.UUID]*RunningTaskGroup),
		order:             make([]uuid.UUID, 0),
	}
}

func (t *TaskGroupManager) ListTaskGroups() []*RunningTaskGroup {
	t.mu.RLock()
	defer t.mu.RUnlock()
	groups := make([]*RunningTaskGroup, 0, len(t.runningTaskGroups))
	for _, id := range t.order {
		if group, ok := t.runningTaskGroups[id]; ok {
			groups = append(groups, group)
		}
	}
	return groups
}

func (t *TaskGroupManager) CreateTaskGroup(name string, tasks []defines.Task, server defines.ControlConfig) (runningTaskGroup *RunningTaskGroup, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ud, err := uuid.NewUUID()
	if err != nil {
		return
	}
	runningTaskGroup = &RunningTaskGroup{
		Name:    name,
		UUID:    ud,
		Tasks:   tasks,
		Control: control.NewControl(server),
	}
	t.runningTaskGroups[ud] = runningTaskGroup
	t.order = append(t.order, ud)
	return runningTaskGroup, nil
}

func (t *TaskGroupManager) GetTaskGroup(ud uuid.UUID) (*RunningTaskGroup, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	group, found := t.runningTaskGroups[ud]
	return group, found
}

func (t *TaskGroupManager) RunTaskGroup(
	ud uuid.UUID,
	ctx context.Context,
	waitWhilePaused func(context.Context) error,
	taskStartCallback func(defines.Task),
	taskSuccessCallback func(defines.Task),
	taskFailedCallback func(defines.Task, error),
) (found bool, err error) {
	group, found := t.GetTaskGroup(ud)
	if !found {
		return found, nil
	}

	control := group.Control
	// 1. 进入前先查ctx，提前终止避免无效操作
	select {
	case <-ctx.Done():
		return true, ctx.Err()
	default:
	}
	err = control.EnterRentalServer()
	if err != nil {
		return true, err
	}
	// 2. 退出时确保清理资源（无论成功/失败/终止）
	defer control.LeaveRentalServer() // 补退出逻辑，成对释放

	// 3. 加任务前查ctx，避免白加
	select {
	case <-ctx.Done():
		return true, ctx.Err()
	default:
	}
	taskIDs := make([]int, 0, len(group.Tasks))
	for _, task := range group.Tasks {
		control.AddTask(task)
		taskIDs = append(taskIDs, task.ID())
	}

	// 4. 遍历任务核心优化：优先响应ctx，执行中也能终止
	for _, taskID := range taskIDs {
		// 先查ctx，直接终止循环，不启动后续任务
		select {
		case <-ctx.Done():
			// 返回终止错误
			return true, ctx.Err()
		default:
		}

		task := control.TaskManager().GetTask(taskID)
		if task == nil {
			continue
		}

		if waitWhilePaused != nil {
			if err := waitWhilePaused(ctx); err != nil {
				control.DeleteTask(task)
				return true, err
			}
		}

		if taskStartCallback != nil {
			taskStartCallback(task)
		}

		// 执行任务：若task.Start支持ctx，直接传；不支持则包一层超时/终止监听
		err := task.Start(ctx) // 优先让任务自身响应ctx，最精准
		// 5. 任务执行后/中，若ctx触发，仍终止后续流程
		if ctx.Err() != nil {
			control.DeleteTask(task)
			return true, ctx.Err()
		}

		// 回调+清理当前任务
		if err != nil {
			if taskFailedCallback != nil {
				taskFailedCallback(task, err)
			}
			control.DeleteTask(task)
			return true, err
		} else {
			if taskSuccessCallback != nil {
				taskSuccessCallback(task)
			}
		}
		control.DeleteTask(task)
	}

	return true, nil // 显式返回，避免默认零值歧义
}

func (t *TaskGroupManager) DeleteTaskGroup(ud uuid.UUID) (found bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, found = t.runningTaskGroups[ud]
	if !found {
		return found
	}
	delete(t.runningTaskGroups, ud)
	for i, id := range t.order {
		if id == ud {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
	return
}
