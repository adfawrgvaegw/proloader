package services

import (
	buildingpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/building"
	"github.com/Yeah114/Fatalder/cmd/grpc/pb/define/interrupt_config"
	"github.com/Yeah114/Fatalder/cmd/grpc/pb/define/server_config"
	"github.com/Yeah114/Fatalder/cmd/grpc/pb/define/tasks_config"
	taskgrouppb "github.com/Yeah114/Fatalder/cmd/grpc/pb/task_group_manager"
	"github.com/Yeah114/Fatalder/utils/data_manager"
	"google.golang.org/grpc"
)

func RegisterConfigServices(server *grpc.Server, dm *data_manager.DataManager) {
	server_config.RegisterServerConfigServiceServer(server, NewServerConfigService(dm))
	interrupt_config.RegisterInterruptConfigServiceServer(server, NewInterruptConfigService(dm))
	tasks_config.RegisterTasksConfigServiceServer(server, NewTasksConfigService(dm))
	buildingpb.RegisterBuildingServiceServer(server, NewBuildingService(dm))
}

func RegisterTaskGroupManagerService(server *grpc.Server) {
	taskgrouppb.RegisterTaskGroupManagerServiceServer(server, NewTaskGroupManagerService())
}
