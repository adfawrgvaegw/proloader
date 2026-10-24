package services

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	interruptpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/interrupt_config"
	serverconfigpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/server_config"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils/data_manager"
	"github.com/google/uuid"
)

type InterruptConfigService struct {
	interruptpb.UnimplementedInterruptConfigServiceServer
	dm *data_manager.DataManager
}

func NewInterruptConfigService(dm *data_manager.DataManager) *InterruptConfigService {
	return &InterruptConfigService{dm: dm}
}

func (s *InterruptConfigService) ListInterruptConfigs(ctx context.Context, _ *reqpb.EmptyRequest) (*interruptpb.ListInterruptConfigsResponse, error) {
	resp := &interruptpb.ListInterruptConfigsResponse{}
	cfgs, err := s.dm.ListInterruptConfigs()
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
		cfg := cfgs[name]
		resp.Configs = append(resp.Configs, &interruptpb.InterruptConfigEntry{
			Name:   name,
			Config: toProtoInterruptConfig(cfg),
		})
	}
	return resp, nil
}

func (s *InterruptConfigService) GetInterruptConfig(ctx context.Context, req *interruptpb.GetInterruptConfigRequest) (*interruptpb.GetInterruptConfigResponse, error) {
	resp := &interruptpb.GetInterruptConfigResponse{}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		resp.Error = "config name required"
		return resp, nil
	}

	cfg, ok, err := s.dm.GetInterruptConfig(name)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	if !ok {
		resp.Error = "config not found"
		return resp, nil
	}

	resp.Config = toProtoInterruptConfig(cfg)
	return resp, nil
}

func (s *InterruptConfigService) SaveInterruptConfig(ctx context.Context, req *interruptpb.SaveInterruptConfigRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetName())
	cfg := req.GetConfig()
	if name == "" && cfg != nil {
		name = strings.TrimSpace(cfg.GetName())
	}
	if name == "" {
		return &respb.StringResponse{Error: "config name required"}, nil
	}
	if cfg == nil {
		return &respb.StringResponse{Error: "config payload required"}, nil
	}

	id := strings.TrimSpace(cfg.GetUuid())
	var parsed uuid.UUID
	var err error
	if id == "" {
		parsed = uuid.New()
	} else {
		parsed, err = uuid.Parse(id)
		if err != nil {
			return &respb.StringResponse{Error: fmt.Sprintf("invalid uuid: %v", err)}, nil
		}
	}

	store := defines.InterruptConfig{
		Name: name,
		UUID: parsed,
		Server: func() defines.ControlConfig {
			if cfg.GetServer() == nil {
				return defines.ControlConfig{}
			}
			return fromProtoServerConfig(cfg.GetServer())
		}(),
		Data: append([]string(nil), cfg.GetData()...),
	}
	if err := s.dm.SaveInterruptConfig(name, store); err != nil {
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("saved interrupt config %s", name)}, nil
}

func (s *InterruptConfigService) DeleteInterruptConfig(ctx context.Context, req *interruptpb.DeleteInterruptConfigRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return &respb.StringResponse{Error: "config name required"}, nil
	}
	err := s.dm.DeleteInterruptConfig(name)
	if err != nil {
		if os.IsNotExist(err) {
			return &respb.StringResponse{Result: fmt.Sprintf("config %s not found", name)}, nil
		}
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("deleted interrupt config %s", name)}, nil
}

func toProtoInterruptConfig(cfg defines.InterruptConfig) *interruptpb.InterruptConfig {
	return &interruptpb.InterruptConfig{
		Name: cfg.Name,
		Uuid: cfg.UUID.String(),
		Server: func() *serverconfigpb.ServerConfig {
			return toProtoServerConfig(cfg.Server)
		}(),
		Data: append([]string(nil), cfg.Data...),
	}
}
