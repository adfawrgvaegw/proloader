package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	taskspb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/tasks"
	taskscfgpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/tasks_config"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	taskpkg "github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils/data_manager"
)

type TasksConfigService struct {
	taskscfgpb.UnimplementedTasksConfigServiceServer
	dm *data_manager.DataManager
}

func NewTasksConfigService(dm *data_manager.DataManager) *TasksConfigService {
	return &TasksConfigService{dm: dm}
}

func (s *TasksConfigService) ListTasksConfigs(ctx context.Context, _ *reqpb.EmptyRequest) (*taskscfgpb.ListTasksConfigsResponse, error) {
	resp := &taskscfgpb.ListTasksConfigsResponse{}
	cfgs, err := s.dm.ListTasksConfigs()
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	names := make([]string, 0, len(cfgs))
	for name := range cfgs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := &taskscfgpb.TasksConfigEntry{Name: name}
		for _, t := range cfgs[name] {
			pbTask, err := taskToProto(t)
			if err != nil {
				resp.Error = fmt.Sprintf("convert task for %s: %v", name, err)
				return resp, nil
			}
			entry.Tasks = append(entry.Tasks, pbTask)
		}
		resp.Configs = append(resp.Configs, entry)
	}
	return resp, nil
}

func (s *TasksConfigService) GetTasksConfig(ctx context.Context, req *taskscfgpb.GetTasksConfigRequest) (*taskscfgpb.GetTasksConfigResponse, error) {
	resp := &taskscfgpb.GetTasksConfigResponse{}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		resp.Error = "config name required"
		return resp, nil
	}
	tasks, ok, err := s.dm.GetTasksConfig(name)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	if !ok {
		resp.Error = "config not found"
		return resp, nil
	}

	for _, t := range tasks {
		pbTask, err := taskToProto(t)
		if err != nil {
			resp.Error = fmt.Sprintf("convert task: %v", err)
			return resp, nil
		}
		resp.Tasks = append(resp.Tasks, pbTask)
	}
	return resp, nil
}

func (s *TasksConfigService) SaveTasksConfig(ctx context.Context, req *taskscfgpb.SaveTasksConfigRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return &respb.StringResponse{Error: "config name required"}, nil
	}
	if len(req.GetTasks()) == 0 {
		return &respb.StringResponse{Error: "at least one task required"}, nil
	}

	defTasks := make([]defines.Task, 0, len(req.GetTasks()))
	for _, pbTask := range req.GetTasks() {
		t, err := taskFromProto(pbTask)
		if err != nil {
			return &respb.StringResponse{Error: fmt.Sprintf("convert task: %v", err)}, nil
		}
		defTasks = append(defTasks, t)
	}

	if err := s.dm.SaveTasksConfig(name, defTasks); err != nil {
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("saved tasks config %s with %d tasks", name, len(defTasks))}, nil
}

func (s *TasksConfigService) DeleteTasksConfig(ctx context.Context, req *taskscfgpb.DeleteTasksConfigRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return &respb.StringResponse{Error: "config name required"}, nil
	}
	err := s.dm.DeleteTasksConfig(name)
	if err != nil {
		if os.IsNotExist(err) {
			return &respb.StringResponse{Result: fmt.Sprintf("config %s not found", name)}, nil
		}
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("deleted tasks config %s", name)}, nil
}

func taskToProto(task defines.Task) (*taskspb.Task, error) {
	switch v := task.(type) {
	case *taskpkg.CommandTask:
		return &taskspb.Task{
			Type: taskspb.TaskType_TASK_TYPE_COMMAND,
			Config: &taskspb.Task_Command{Command: &taskspb.CommandTaskConfig{
				Command:  v.Command,
				Identity: v.Identity,
			}},
		}, nil
	case *taskpkg.SleepTask:
		return &taskspb.Task{
			Type:   taskspb.TaskType_TASK_TYPE_SLEEP,
			Config: &taskspb.Task_Sleep{Sleep: &taskspb.SleepTaskConfig{Duration: int32(v.Duration)}},
		}, nil
	case *taskpkg.ChatTask:
		return &taskspb.Task{
			Type:   taskspb.TaskType_TASK_TYPE_CHAT,
			Config: &taskspb.Task_Chat{Chat: &taskspb.ChatTaskConfig{Content: v.Content}},
		}, nil
	case *taskpkg.MoveTask:
		return &taskspb.Task{
			Type: taskspb.TaskType_TASK_TYPE_MOVE,
			Config: &taskspb.Task_Move{Move: &taskspb.MoveTaskConfig{
				Pos: blockPosToProto(v.Pos),
			}},
		}, nil
	case *taskpkg.BuildTask:
		cfg := &taskspb.BuildTaskConfig{
			FilePath:             v.FilePath,
			StartPos:             blockPosToProto(v.StartPos),
			DimensionName:        v.DimensionName,
			Speed:                int32(v.Speed),
			ChunkGroupSide:       int32(v.ChunkGroupSide),
			AutoCleanBlock:       v.AutoCleanBlock,
			AutoCleanItem:        v.AutoCleanItem,
			AutoDisableCommand:   v.AutoDisableCommand,
			EnableDeltaMode:      v.EnableDeltaMode,
			IgnoreCommandBlock:   v.IgnoreCommandBlock,
			IgnoreOtherNbtBlock:  v.IgnoreOtherNBTBlock,
			AutoUpgradeCommand:   v.AutoUpgradeCommand,
			WaitChunkLoad:        v.WaitChunkLoad,
			ConvertToMcworld:     v.ConvertToMCWorld,
			ConsoleWorldPos:      blockPosToProto(v.ConsoleWorldPos),
			ShowNbtProgress:      v.ShowNBTProgress,
			VerifyAfterChunk:     int32(v.VerifyAfterChunk),
			VerifyChunkLevel:     int32(v.VerifyChunkLevel),
			AutoEnterFixMode:     v.AutoEnterFixMode,
			EnterFixModeDirectly: v.EnterFixModeDirectly,
			Progress:             int32(v.Progress),
		}
		return &taskspb.Task{
			Type:   taskspb.TaskType_TASK_TYPE_BUILD,
			Config: &taskspb.Task_Build{Build: cfg},
		}, nil
	case *taskpkg.ExportTask:
		cfg := &taskspb.ExportTaskConfig{
			FilePath:      v.FilePath,
			FileType:      v.FileType,
			DimensionName: v.DimensionName,
			StartPos:      blockPosToProto(v.StartPos),
			EndPos:        blockPosToProto(v.EndPos),
		}
		return &taskspb.Task{
			Type:   taskspb.TaskType_TASK_TYPE_EXPORT,
			Config: &taskspb.Task_Export{Export: cfg},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported task type %T", task)
	}
}

func taskFromProto(pbTask *taskspb.Task) (defines.Task, error) {
	if pbTask == nil {
		return nil, errors.New("task payload is nil")
	}
	switch pbTask.GetType() {
	case taskspb.TaskType_TASK_TYPE_COMMAND:
		cfg := pbTask.GetCommand()
		if cfg == nil {
			return nil, errors.New("command config required")
		}
		t := taskpkg.NewCommandTask()
		t.Command = cfg.GetCommand()
		t.Identity = cfg.GetIdentity()
		return t, nil
	case taskspb.TaskType_TASK_TYPE_SLEEP:
		cfg := pbTask.GetSleep()
		if cfg == nil {
			return nil, errors.New("sleep config required")
		}
		t := taskpkg.NewSleepTask()
		t.Duration = int(cfg.GetDuration())
		return t, nil
	case taskspb.TaskType_TASK_TYPE_CHAT:
		cfg := pbTask.GetChat()
		if cfg == nil {
			return nil, errors.New("chat config required")
		}
		t := taskpkg.NewChatTask()
		t.Content = cfg.GetContent()
		return t, nil
	case taskspb.TaskType_TASK_TYPE_MOVE:
		cfg := pbTask.GetMove()
		if cfg == nil {
			return nil, errors.New("move config required")
		}
		t := taskpkg.NewMoveTask()
		t.Pos = protoToBlockPos(cfg.GetPos())
		return t, nil
	case taskspb.TaskType_TASK_TYPE_BUILD:
		cfg := pbTask.GetBuild()
		if cfg == nil {
			return nil, errors.New("build config required")
		}
		t := taskpkg.NewBuildTask()
		t.FilePath = cfg.GetFilePath()
		t.StartPos = protoToBlockPos(cfg.GetStartPos())
		t.DimensionName = cfg.GetDimensionName()
		t.Speed = int(cfg.GetSpeed())
		t.ChunkGroupSide = int(cfg.GetChunkGroupSide())
		t.AutoCleanBlock = cfg.GetAutoCleanBlock()
		t.AutoCleanItem = cfg.GetAutoCleanItem()
		t.AutoDisableCommand = cfg.GetAutoDisableCommand()
		t.EnableDeltaMode = cfg.GetEnableDeltaMode()
		t.IgnoreCommandBlock = cfg.GetIgnoreCommandBlock()
		t.IgnoreOtherNBTBlock = cfg.GetIgnoreOtherNbtBlock()
		t.AutoUpgradeCommand = cfg.GetAutoUpgradeCommand()
		t.WaitChunkLoad = cfg.GetWaitChunkLoad()
		t.ConvertToMCWorld = cfg.GetConvertToMcworld()
		t.ConsoleWorldPos = protoToBlockPos(cfg.GetConsoleWorldPos())
		t.ShowNBTProgress = cfg.GetShowNbtProgress()
		t.VerifyAfterChunk = int(cfg.GetVerifyAfterChunk())
		t.VerifyChunkLevel = int(cfg.GetVerifyChunkLevel())
		t.AutoEnterFixMode = cfg.GetAutoEnterFixMode()
		t.EnterFixModeDirectly = cfg.GetEnterFixModeDirectly()
		t.Progress = int(cfg.GetProgress())
		return t, nil
	case taskspb.TaskType_TASK_TYPE_EXPORT:
		cfg := pbTask.GetExport()
		if cfg == nil {
			return nil, errors.New("export config required")
		}
		t := taskpkg.NewExportTask()
		t.FilePath = cfg.GetFilePath()
		t.FileType = cfg.GetFileType()
		t.DimensionName = cfg.GetDimensionName()
		t.StartPos = protoToBlockPos(cfg.GetStartPos())
		t.EndPos = protoToBlockPos(cfg.GetEndPos())
		return t, nil
	default:
		return nil, fmt.Errorf("unsupported task type %s", pbTask.GetType().String())
	}
}

func blockPosToProto(pos defines.BlockPos) *taskspb.BlockPos {
	return &taskspb.BlockPos{X: pos.X(), Y: pos.Y(), Z: pos.Z()}
}

func protoToBlockPos(pbPos *taskspb.BlockPos) defines.BlockPos {
	if pbPos == nil {
		return defines.BlockPos{}
	}
	return defines.BlockPos{pbPos.GetX(), pbPos.GetY(), pbPos.GetZ()}
}
