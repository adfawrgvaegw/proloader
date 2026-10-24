package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	bwochunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	taskspb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/tasks"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	taskgrouppb "github.com/Yeah114/Fatalder/cmd/grpc/pb/task_group_manager"
	"github.com/Yeah114/Fatalder/cmd/grpc/task_group_manager"
	taskpkg "github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsstructure "github.com/Yeah114/WaterStructure/structure"
	"github.com/google/uuid"
)

type TaskGroupManagerService struct {
	taskgrouppb.UnimplementedTaskGroupManagerServiceServer
	manager      *task_group_manager.TaskGroupManager
	runningMu    sync.Mutex
	running      map[uuid.UUID]*taskExecution
	eventStoreMu sync.RWMutex
	eventStore   map[uuid.UUID][]*taskspb.TaskEvent
	eventsHub    *taskEventHub
}

func NewTaskGroupManagerService() *TaskGroupManagerService {
	return &TaskGroupManagerService{
		manager:    task_group_manager.NewTaskGroupManager(),
		running:    make(map[uuid.UUID]*taskExecution),
		eventStore: make(map[uuid.UUID][]*taskspb.TaskEvent),
		eventsHub:  newTaskEventHub(),
	}
}

func (s *TaskGroupManagerService) PushTaskEvent(_ *reqpb.EmptyRequest, stream taskgrouppb.TaskGroupManagerService_PushTaskEventServer) error {
	id, events := s.eventsHub.subscribe()
	defer s.eventsHub.unsubscribe(id)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case evt, ok := <-events:
			if !ok {
				return nil
			}
			if evt == nil {
				continue
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		}
	}
}

func (s *TaskGroupManagerService) ListRunningTaskGroup(ctx context.Context, _ *reqpb.EmptyRequest) (*taskgrouppb.ListRunningTaskGroupResponse, error) {
	resp := &taskgrouppb.ListRunningTaskGroupResponse{}
	groups := s.manager.ListTaskGroups()
	for i := len(groups) - 1; i >= 0; i-- {
		group := groups[i]
		pbGroup, err := s.toProtoGroup(group)
		if err != nil {
			resp.Error = err.Error()
			return resp, nil
		}
		resp.Groups = append(resp.Groups, pbGroup)
	}
	return resp, nil
}

func (s *TaskGroupManagerService) CreateTaskGroup(ctx context.Context, req *taskgrouppb.CreateTaskGroupRequest) (*taskgrouppb.CreateTaskGroupResponse, error) {
	resp := &taskgrouppb.CreateTaskGroupResponse{}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		resp.Error = "task group name required"
		return resp, nil
	}
	if len(req.GetTasks()) == 0 {
		resp.Error = "at least one task required"
		return resp, nil
	}
	serverCfg := req.GetServer()
	if serverCfg == nil {
		resp.Error = "server config required"
		return resp, nil
	}

	tasks := make([]defines.Task, 0, len(req.GetTasks()))
	for _, pbTask := range req.GetTasks() {
		t, err := taskFromProto(pbTask)
		if err != nil {
			resp.Error = fmt.Sprintf("convert task: %v", err)
			return resp, nil
		}
		tasks = append(tasks, t)
	}

	group, err := s.manager.CreateTaskGroup(name, tasks, fromProtoServerConfig(serverCfg))
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	resp.Group, err = s.toProtoGroup(group)
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *TaskGroupManagerService) RunTaskGroup(ctx context.Context, req *reqpb.StringRequest) (*respb.BoolResponse, error) {
	id, group, err := s.requireGroup(req.GetData())
	if err != nil {
		return &respb.BoolResponse{Error: err.Error()}, nil
	}
	for _, task := range group.Tasks {
		s.bindTaskEvents(group.UUID, group.Name, task)
	}

	s.runningMu.Lock()
	if _, exists := s.running[id]; exists {
		s.runningMu.Unlock()
		return &respb.BoolResponse{Error: "task group is already running"}, nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	exec := &taskExecution{ctx: runCtx, cancel: cancel}
	s.running[id] = exec
	s.runningMu.Unlock()

	go s.executeTaskGroup(id, group, exec)
	return &respb.BoolResponse{Result: true}, nil
}

func (s *TaskGroupManagerService) GetTaskGroup(ctx context.Context, req *reqpb.StringRequest) (*taskgrouppb.GetTaskGroupResponse, error) {
	resp := &taskgrouppb.GetTaskGroupResponse{}
	_, group, err := s.requireGroup(req.GetData())
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	pbGroup, convErr := s.toProtoGroup(group)
	if convErr != nil {
		resp.Error = convErr.Error()
		return resp, nil
	}
	resp.Group = pbGroup
	return resp, nil
}

func (s *TaskGroupManagerService) PauseTaskGroup(ctx context.Context, req *reqpb.StringRequest) (*respb.BoolResponse, error) {
	id, group, err := s.requireGroup(req.GetData())
	if err != nil {
		return &respb.BoolResponse{Error: err.Error()}, nil
	}
	exec := s.getExecution(id)
	if exec == nil {
		return &respb.BoolResponse{Error: "task group is not running"}, nil
	}
	if err := exec.pause(); err != nil {
		return &respb.BoolResponse{Error: err.Error()}, nil
	}
	s.recordEvent(id, newSimpleTaskEvent(group.UUID.String(), taskspb.TaskEventType_TASK_PAUSE, fmt.Sprintf("[%s] 当前任务已暂停", group.Name)))
	return &respb.BoolResponse{Result: true}, nil
}

func (s *TaskGroupManagerService) ResumeTaskGroup(ctx context.Context, req *reqpb.StringRequest) (*respb.BoolResponse, error) {
	id, group, err := s.requireGroup(req.GetData())
	if err != nil {
		return &respb.BoolResponse{Error: err.Error()}, nil
	}
	exec := s.getExecution(id)
	if exec == nil {
		return &respb.BoolResponse{Error: "task group is not running"}, nil
	}
	if err := exec.resume(); err != nil {
		return &respb.BoolResponse{Error: err.Error()}, nil
	}
	s.recordEvent(id, newSimpleTaskEvent(group.UUID.String(), taskspb.TaskEventType_TASK_RESUME, fmt.Sprintf("[%s] 当前任务已恢复", group.Name)))
	return &respb.BoolResponse{Result: true}, nil
}

func (s *TaskGroupManagerService) StopTaskGroup(ctx context.Context, req *reqpb.StringRequest) (*respb.BoolResponse, error) {
	id, group, err := s.requireGroup(req.GetData())
	if err != nil {
		return &respb.BoolResponse{Error: err.Error()}, nil
	}
	s.runningMu.Lock()
	exec := s.running[id]
	if exec != nil {
		exec.stop()
		delete(s.running, id)
	}
	s.runningMu.Unlock()
	s.manager.DeleteTaskGroup(id)
	groupID := group.UUID.String()
	status := "任务组已停止"
	if exec != nil {
		status = "任务组执行已被终止"
	}
	s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_STOP, fmt.Sprintf("[%s] %s", group.Name, status)))
	return &respb.BoolResponse{Result: true}, nil
}

func (s *TaskGroupManagerService) executeTaskGroup(id uuid.UUID, group *task_group_manager.RunningTaskGroup, exec *taskExecution) {
	groupID := group.UUID.String()
	s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_START, fmt.Sprintf("[%s] 任务组开始执行", group.Name)))
	_, err := s.manager.RunTaskGroup(
		id,
		exec.ctx,
		exec.waitWhilePaused,
		func(task defines.Task) {
			exec.setCurrentTask(task)
			taskID, taskType := describeTask(task)
			s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_START, fmt.Sprintf("[%s] 任务 %s (#%d) 开始执行", group.Name, taskType, taskID)))
		},
		func(task defines.Task) {
			taskID, taskType := describeTask(task)
			s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_SUCCESS, fmt.Sprintf("[%s] 任务 %s (#%d) 已完成", group.Name, taskType, taskID)))
			exec.clearCurrentTask()
		},
		func(task defines.Task, runErr error) {
			taskID, taskType := describeTask(task)
			s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_FAILED, fmt.Sprintf("[%s] 任务 %s (#%d) 执行失败: %v", group.Name, taskType, taskID, runErr)))
			exec.clearCurrentTask()
		},
	)
	s.runningMu.Lock()
	delete(s.running, id)
	s.runningMu.Unlock()
	s.handleGroupCompletion(id, group, err)
}

func (s *TaskGroupManagerService) bindTaskEvents(groupUUID uuid.UUID, groupName string, task defines.Task) {
	switch t := task.(type) {
	case *taskpkg.BuildTask:
		s.bindBuildTaskEvents(groupUUID, groupName, t)
	case *taskpkg.ExportTask:
		s.bindExportTaskEvents(groupUUID, groupName, t)
	}
}

func (s *TaskGroupManagerService) bindBuildTaskEvents(groupUUID uuid.UUID, groupName string, task *taskpkg.BuildTask) {
	var chunkCompleted atomic.Int32
	var chunkTotal atomic.Int32
	var convertTotal atomic.Int32
	var convertCompleted atomic.Int32
	groupID := groupUUID.String()

	emit := func(evt *taskspb.TaskEvent) {
		if evt != nil {
			s.recordEvent(groupUUID, evt)
		}
	}

	structureEvent := func(eventType taskspb.TaskEventType, prefix string, info wsstructure.Structure) {
		if info == nil {
			return
		}
		pbInfo, totalChunks := buildStructureInfoToProto(info)
		if totalChunks > 0 {
			chunkTotal.Store(totalChunks)
		}
		evt := newTaskEvent(groupID, task, eventType, fmt.Sprintf("[%s] %s: %s", groupName, prefix, info.Name()))
		evt.Event = &taskspb.TaskEvent_BuildStructure{BuildStructure: pbInfo}
		emit(evt)
	}

	task.SetStartCallback(func() {
		emit(newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_START, fmt.Sprintf("[%s] 构建任务开始准备", groupName)))
	})

	task.SetGotTypeCallback(func(info wsstructure.Structure) {
		structureEvent(taskspb.TaskEventType_TASK_BUILD_GOT_TYPE, "获取结构类型", info)
	})

	task.SetParsingCallback(func() {
		emit(newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_PARSING, fmt.Sprintf("[%s] 正在解析结构文件", groupName)))
	})

	task.SetParsedCallback(func(info wsstructure.Structure) {
		structureEvent(taskspb.TaskEventType_TASK_BUILD_PARSED, "解析完成", info)
	})

	task.SetStartBuildCallback(func() {
		emit(newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_START_BUILDING, fmt.Sprintf("[%s] 开始正式构建", groupName)))
	})

	task.SetEveryChunkBuiltCallback(func(pos defines.ChunkPos, _ *bwochunk.Chunk) {
		completed := chunkCompleted.Add(1)
		total := chunkTotal.Load()
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_EVERY_CHUNK_BUILT, fmt.Sprintf("[%s] 完成区块 (%d,%d)，进度 %d/%d", groupName, pos.X(), pos.Z(), completed, total))
		evt.Event = &taskspb.TaskEvent_BuildChunk{BuildChunk: &taskspb.BuildChunkEvent{
			ChunkPos:        taskChunkPosToProto(pos),
			CompletedChunks: completed,
			TotalChunks:     total,
		}}
		emit(evt)
	})

	task.SetEveryChunkAutoFixCallback(func(pos defines.ChunkPos) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_EVERY_CHUNK_AUTO_FIX, fmt.Sprintf("[%s] 自动修补区块 (%d,%d)", groupName, pos.X(), pos.Z()))
		evt.Event = &taskspb.TaskEvent_BuildChunk{BuildChunk: &taskspb.BuildChunkEvent{ChunkPos: taskChunkPosToProto(pos)}}
		emit(evt)
	})

	task.SetChunkFixExceededLimitCallback(func(pos defines.ChunkPos, attempts int) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CHUNK_FIX_EXCEEDED_LIMIT, fmt.Sprintf("[%s] 区块 (%d,%d) 修补次数超过上限: %d", groupName, pos.X(), pos.Z(), attempts))
		evt.Event = &taskspb.TaskEvent_BuildChunkFixExceeded{BuildChunkFixExceeded: &taskspb.BuildChunkFixExceededEvent{
			ChunkPos: taskChunkPosToProto(pos),
			Attempts: int32(attempts),
		}}
		emit(evt)
	})

	task.SetEveryNBTBlockBuildingCallback(func(pos defines.BlockPos, runtimeID uint32, nbt map[string]any) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_EVERY_NBT_BLOCK_BUILDING, fmt.Sprintf("[%s] 放置 NBT 方块 (%d,%d,%d)", groupName, pos.X(), pos.Y(), pos.Z()))
		evt.Event = &taskspb.TaskEvent_BuildNbtBlock{BuildNbtBlock: &taskspb.BuildNBTBlockEvent{
			BlockPos:       taskBlockPosToProto(pos),
			BlockRuntimeId: runtimeID,
			NbtJson:        mapToJSON(nbt),
		}}
		emit(evt)
	})

	task.SetChargeCostSuccessCallback(func(deducted int, balance string, info taskpkg.BlockCountInfo) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CHARGE_COST_SUCCESS, fmt.Sprintf("[%s] 扣费成功，消耗 %d，余额 %s", groupName, deducted, balance))
		evt.Event = &taskspb.TaskEvent_BuildChargeCost{BuildChargeCost: &taskspb.BuildChargeCostEvent{
			BlockInfo: toProtoBlockCountInfo(info),
			Deducted:  int32(deducted),
			Balance:   balance,
		}}
		emit(evt)
	})

	task.SetChargeCostDuplicateCallback(func(info taskpkg.BlockCountInfo) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CHARGE_COST_DUPLICATE, fmt.Sprintf("[%s] 扣费已处理，跳过重复请求", groupName))
		evt.Event = &taskspb.TaskEvent_BuildChargeDuplicate{BuildChargeDuplicate: &taskspb.BuildChargeCostDuplicateEvent{
			BlockInfo: toProtoBlockCountInfo(info),
		}}
		emit(evt)
	})

	task.SetChargeCostFailedCallback(func(message string, info taskpkg.BlockCountInfo) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CHARGE_COST_FAILED, fmt.Sprintf("[%s] 扣费失败: %s", groupName, message))
		evt.Event = &taskspb.TaskEvent_BuildChargeFailed{BuildChargeFailed: &taskspb.BuildChargeCostFailedEvent{
			BlockInfo: toProtoBlockCountInfo(info),
			Error:     message,
		}}
		emit(evt)
	})

	task.SetUserCenterNotInitCallback(func() {
		emit(newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_USER_CENTER_NOT_INIT, fmt.Sprintf("[%s] 用户中心未初始化", groupName)))
	})

	task.SetEnterFixModeCallback(func() {
		emit(newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_ENTER_FIX_MODE, fmt.Sprintf("[%s] 进入修补模式", groupName)))
	})

	task.SetExitFixModeCallback(func() {
		emit(newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_EXIT_FIX_MODE, fmt.Sprintf("[%s] 退出修补模式", groupName)))
	})

	task.SetEveryChunkFixStartCallback(func(pos defines.ChunkPos, player string) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_EVERY_CHUNK_FIX_START, fmt.Sprintf("[%s] 玩家 %s 开始修补区块 (%d,%d)", groupName, player, pos.X(), pos.Z()))
		evt.Event = &taskspb.TaskEvent_BuildChunkFix{BuildChunkFix: &taskspb.BuildChunkFixEvent{
			ChunkPos:   taskChunkPosToProto(pos),
			PlayerName: player,
		}}
		emit(evt)
	})

	task.SetEveryChunkFixEndCallback(func(pos defines.ChunkPos, player string) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_EVERY_CHUNK_FIX_END, fmt.Sprintf("[%s] 玩家 %s 修补区块 (%d,%d) 完成", groupName, player, pos.X(), pos.Z()))
		evt.Event = &taskspb.TaskEvent_BuildChunkFix{BuildChunkFix: &taskspb.BuildChunkFixEvent{
			ChunkPos:   taskChunkPosToProto(pos),
			PlayerName: player,
		}}
		emit(evt)
	})

	task.SetConvertStartCallback(func(srcPath, destPath string) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CONVERT_START, fmt.Sprintf("[%s] 开始转换为 MCWorld", groupName))
		evt.Event = &taskspb.TaskEvent_BuildConvertStart{BuildConvertStart: &taskspb.BuildConvertStartEvent{
			SrcPath:  srcPath,
			DestPath: destPath,
		}}
		emit(evt)
	})

	task.SetConvertExistsCallback(func(path string) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CONVERT_EXISTS, fmt.Sprintf("[%s] MCWorld 已存在: %s", groupName, path))
		evt.Event = &taskspb.TaskEvent_BuildConvertExists{BuildConvertExists: &taskspb.BuildConvertExistsEvent{Path: path}}
		emit(evt)
	})

	task.SetConvertSuccessCallback(func(path string) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CONVERT_SUCCESS, fmt.Sprintf("[%s] MCWorld 转换完成: %s", groupName, path))
		evt.Event = &taskspb.TaskEvent_BuildConvertSuccess{BuildConvertSuccess: &taskspb.BuildConvertSuccessEvent{Path: path}}
		emit(evt)
	})

	task.SetConvertProgressInitCallback(func(total int) {
		convertTotal.Store(int32(total))
		convertCompleted.Store(0)
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CONVERT_PROGRESS_INIT, fmt.Sprintf("[%s] MCWorld 转换初始化，总计 %d 步", groupName, total))
		evt.Event = &taskspb.TaskEvent_BuildConvertProgressInit{BuildConvertProgressInit: &taskspb.BuildConvertProgressInitEvent{Total: int32(total)}}
		emit(evt)
	})

	task.SetConvertProgressCallback(func() {
		completed := convertCompleted.Add(1)
		total := convertTotal.Load()
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_BUILD_CONVERT_PROGRESS, fmt.Sprintf("[%s] MCWorld 转换进度 %d/%d", groupName, completed, total))
		evt.Event = &taskspb.TaskEvent_BuildConvertProgress{BuildConvertProgress: &taskspb.BuildConvertProgressEvent{
			Completed: completed,
			Total:     total,
		}}
		emit(evt)
	})
}

func (s *TaskGroupManagerService) bindExportTaskEvents(groupUUID uuid.UUID, groupName string, task *taskpkg.ExportTask) {
	var chunkCompleted atomic.Int32
	var chunkTotal atomic.Int32
	groupID := groupUUID.String()

	emit := func(evt *taskspb.TaskEvent) {
		if evt != nil {
			s.recordEvent(groupUUID, evt)
		}
	}

	task.SetStartCallback(func(size wsdefine.Size) {
		width := size.GetWidth()
		height := size.GetHeight()
		length := size.GetLength()
		total := int32(((width + 15) / 16) * ((length + 15) / 16))
		chunkTotal.Store(total)
		chunkCompleted.Store(0)
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_EXPORT_START, fmt.Sprintf("[%s] 导出任务开始，范围: %dx%dx%d", groupName, width, height, length))
		evt.Event = &taskspb.TaskEvent_ExportStart{ExportStart: &taskspb.ExportStartEvent{Size: taskStructureSizeToProto(size)}}
		emit(evt)
	})

	task.SetEveryChunkExportCallback(func(pos bwodefine.ChunkPos) {
		completed := chunkCompleted.Add(1)
		total := chunkTotal.Load()
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_EXPORT_EVERY_CHUNK_EXPORTED, fmt.Sprintf("[%s] 导出区块 (%d,%d)，进度 %d/%d", groupName, pos.X(), pos.Z(), completed, total))
		evt.Event = &taskspb.TaskEvent_ExportChunk{ExportChunk: &taskspb.ExportChunkEvent{
			ChunkPos:        taskChunkPosToProto(pos),
			CompletedChunks: completed,
			TotalChunks:     total,
		}}
		emit(evt)
	})

	task.SetEverySubChunkSaveFailedCallback(func(pos bwodefine.SubChunkPos, err error) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_EXPORT_EVERY_SUB_CHUNK_SAVE_FAILED, fmt.Sprintf("[%s] 存储子区块 (%d,%d,%d) 失败: %v", groupName, pos.X(), pos.Y(), pos.Z(), err))
		evt.Event = &taskspb.TaskEvent_ExportSubChunkFailed{ExportSubChunkFailed: &taskspb.ExportSubChunkFailedEvent{
			SubChunkPos: taskSubChunkPosToProto(pos),
			Error:       errString(err),
		}}
		emit(evt)
	})

	task.SetEverySubChunkDecodeNBTFailedCallback(func(pos bwodefine.SubChunkPos, err error) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_EXPORT_EVERY_SUB_CHUNK_DECODE_NBT_FAILED, fmt.Sprintf("[%s] 解析子区块 NBT (%d,%d,%d) 失败: %v", groupName, pos.X(), pos.Y(), pos.Z(), err))
		evt.Event = &taskspb.TaskEvent_ExportSubChunkFailed{ExportSubChunkFailed: &taskspb.ExportSubChunkFailedEvent{
			SubChunkPos: taskSubChunkPosToProto(pos),
			Error:       errString(err),
		}}
		emit(evt)
	})

	task.SetEveryChunkNBTSaveFailedCallback(func(pos bwodefine.ChunkPos, err error) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_EXPORT_EVERY_CHUNK_NBT_SAVE_FAILED, fmt.Sprintf("[%s] 保存区块 NBT (%d,%d) 失败: %v", groupName, pos.X(), pos.Z(), err))
		evt.Event = &taskspb.TaskEvent_ExportChunkFailed{ExportChunkFailed: &taskspb.ExportChunkFailedEvent{
			ChunkPos: taskChunkPosToProto(pos),
			Error:    errString(err),
		}}
		emit(evt)
	})

	task.SetFinishedCallback(func(path string) {
		evt := newTaskEvent(groupID, task, taskspb.TaskEventType_TASK_EXPORT_FINISHED, fmt.Sprintf("[%s] 导出完成: %s", groupName, path))
		evt.Event = &taskspb.TaskEvent_ExportFinished{ExportFinished: &taskspb.ExportFinishedEvent{Path: path}}
		emit(evt)
	})
}

func (s *TaskGroupManagerService) handleGroupCompletion(id uuid.UUID, group *task_group_manager.RunningTaskGroup, err error) {
	groupID := group.UUID.String()
	if err == nil {
		s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_SUCCESS, fmt.Sprintf("[%s] 任务组执行完成", group.Name)))
		return
	}
	if errors.Is(err, context.Canceled) {
		s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_STOP, fmt.Sprintf("[%s] 任务组已取消", group.Name)))
		return
	}
	s.recordEvent(id, newSimpleTaskEvent(groupID, taskspb.TaskEventType_TASK_FAILED, fmt.Sprintf("[%s] 任务组执行失败: %v", group.Name, err)))
}

func (s *TaskGroupManagerService) toProtoGroup(group *task_group_manager.RunningTaskGroup) (*taskgrouppb.RunningTaskGroup, error) {
	if group == nil {
		return nil, errors.New("task group is nil")
	}
	pbGroup := &taskgrouppb.RunningTaskGroup{
		Name: group.Name,
		Uuid: group.UUID.String(),
	}
	for _, task := range group.Tasks {
		pbTask, err := taskToProto(task)
		if err != nil {
			return nil, fmt.Errorf("convert task for group %s: %w", group.Name, err)
		}
		pbGroup.Tasks = append(pbGroup.Tasks, pbTask)
	}
	for _, evt := range s.getGroupEvents(group.UUID) {
		pbGroup.Events = append(pbGroup.Events, evt)
	}
	return pbGroup, nil
}

func (s *TaskGroupManagerService) getExecution(id uuid.UUID) *taskExecution {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	return s.running[id]
}

func (s *TaskGroupManagerService) recordEvent(id uuid.UUID, evt *taskspb.TaskEvent) {
	if evt == nil {
		return
	}
	s.eventStoreMu.Lock()
	s.eventStore[id] = append(s.eventStore[id], evt)
	s.eventStoreMu.Unlock()
	s.eventsHub.publish(evt)
}

func (s *TaskGroupManagerService) getGroupEvents(id uuid.UUID) []*taskspb.TaskEvent {
	s.eventStoreMu.RLock()
	defer s.eventStoreMu.RUnlock()
	events := s.eventStore[id]
	result := make([]*taskspb.TaskEvent, len(events))
	copy(result, events)
	return result
}

func (s *TaskGroupManagerService) requireGroup(raw string) (uuid.UUID, *task_group_manager.RunningTaskGroup, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return uuid.UUID{}, nil, errors.New("task group uuid required")
	}
	id, err := uuid.Parse(trimmed)
	if err != nil {
		return uuid.UUID{}, nil, fmt.Errorf("invalid task group uuid: %w", err)
	}
	group, ok := s.manager.GetTaskGroup(id)
	if !ok {
		return uuid.UUID{}, nil, errors.New("task group not found")
	}
	return id, group, nil
}

func newSimpleTaskEvent(groupID string, eventType taskspb.TaskEventType, message string) *taskspb.TaskEvent {
	return &taskspb.TaskEvent{Uuid: groupID, Type: eventType, Message: message}
}

func newTaskEvent(groupID string, task defines.Task, eventType taskspb.TaskEventType, message string) *taskspb.TaskEvent {
	_ = task
	return &taskspb.TaskEvent{Uuid: groupID, Type: eventType, Message: message}
}

func describeTask(task defines.Task) (int, string) {
	if task == nil {
		return 0, "unknown"
	}
	return task.ID(), task.Type()
}

type taskExecution struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	task    defines.Task
	paused  bool
	pauseCh chan struct{}
}

func (e *taskExecution) setCurrentTask(task defines.Task) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.task = task
}

func (e *taskExecution) clearCurrentTask() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.task = nil
}

func (e *taskExecution) waitWhilePaused(ctx context.Context) error {
	for {
		e.mu.Lock()
		ch := e.pauseCh
		e.mu.Unlock()
		if ch == nil {
			return nil
		}
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (e *taskExecution) pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.paused {
		return errors.New("task already paused")
	}
	if e.task != nil {
		if err := e.task.Pause(); err != nil {
			return err
		}
	}
	if e.pauseCh == nil {
		e.pauseCh = make(chan struct{})
	}
	e.paused = true
	return nil
}

func (e *taskExecution) resume() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.paused {
		return errors.New("task is not paused")
	}
	if e.task != nil {
		if err := e.task.Resume(); err != nil {
			return err
		}
	}
	e.paused = false
	if e.pauseCh != nil {
		close(e.pauseCh)
		e.pauseCh = nil
	}
	return nil
}

func (e *taskExecution) stop() {
	e.mu.Lock()
	task := e.task
	pauseCh := e.pauseCh
	e.task = nil
	e.paused = false
	e.pauseCh = nil
	e.mu.Unlock()
	if task != nil {
		_ = task.Stop()
	}
	if pauseCh != nil {
		close(pauseCh)
	}
	if e.cancel != nil {
		e.cancel()
	}
}

type taskEventHub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uint64]chan *taskspb.TaskEvent
}

func newTaskEventHub() *taskEventHub {
	return &taskEventHub{subscribers: make(map[uint64]chan *taskspb.TaskEvent)}
}

func (h *taskEventHub) subscribe() (uint64, <-chan *taskspb.TaskEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan *taskspb.TaskEvent, 32)
	h.subscribers[id] = ch
	return id, ch
}

func (h *taskEventHub) unsubscribe(id uint64) {
	h.mu.Lock()
	ch := h.subscribers[id]
	delete(h.subscribers, id)
	h.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func (h *taskEventHub) publish(evt *taskspb.TaskEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}

type chunkPosition interface {
	X() int32
	Z() int32
}

type blockPosition interface {
	X() int32
	Y() int32
	Z() int32
}

type subChunkPosition interface {
	X() int32
	Y() int32
	Z() int32
}

func taskChunkPosToProto(pos chunkPosition) *taskspb.ChunkPos {
	if pos == nil {
		return nil
	}
	return &taskspb.ChunkPos{X: pos.X(), Z: pos.Z()}
}

func taskBlockPosToProto(pos blockPosition) *taskspb.BlockPos {
	if pos == nil {
		return nil
	}
	return &taskspb.BlockPos{X: pos.X(), Y: pos.Y(), Z: pos.Z()}
}

func taskSubChunkPosToProto(pos subChunkPosition) *taskspb.SubChunkPos {
	if pos == nil {
		return nil
	}
	return &taskspb.SubChunkPos{X: pos.X(), Y: pos.Y(), Z: pos.Z()}
}

func taskStructureSizeToProto(size wsdefine.Size) *taskspb.StructureSize {
	return &taskspb.StructureSize{
		Width:  int32(size.GetWidth()),
		Height: int32(size.GetHeight()),
		Length: int32(size.GetLength()),
	}
}

func buildStructureInfoToProto(info wsstructure.Structure) (*taskspb.BuildStructureInfo, int32) {
	if info == nil {
		return nil, 0
	}
	size := info.GetSize()
	return &taskspb.BuildStructureInfo{
		Name: info.Name(),
		Size: taskStructureSizeToProto(size),
	}, int32(size.GetChunkCount())
}

func toProtoBlockCountInfo(info taskpkg.BlockCountInfo) *taskspb.BlockCountInfo {
	return &taskspb.BlockCountInfo{
		BlockCount:        int32(info.BlockCount),
		CommandBlockCount: int32(info.CommandBlockCount),
		OtherNbtCount:     int32(info.OtherNBTCount),
	}
}

func mapToJSON(m map[string]any) string {
	if m == nil {
		return ""
	}
	bs, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(bs)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
