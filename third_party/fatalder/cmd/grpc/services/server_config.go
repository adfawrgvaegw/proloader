package services

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	serverconfigpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/server_config"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils/data_manager"
)

type ServerConfigService struct {
	serverconfigpb.UnimplementedServerConfigServiceServer
	dm *data_manager.DataManager
}

func NewServerConfigService(dm *data_manager.DataManager) *ServerConfigService {
	return &ServerConfigService{dm: dm}
}

func (s *ServerConfigService) ListServerConfigs(ctx context.Context, _ *reqpb.EmptyRequest) (*serverconfigpb.ListServerConfigsResponse, error) {
	resp := &serverconfigpb.ListServerConfigsResponse{}
	configs, err := s.dm.ListServerConfigs()
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cfg := configs[name]
		resp.Configs = append(resp.Configs, &serverconfigpb.ServerConfigEntry{
			Name:   name,
			Config: toProtoServerConfig(cfg),
		})
	}
	return resp, nil
}

func (s *ServerConfigService) GetServerConfig(ctx context.Context, req *serverconfigpb.GetServerConfigRequest) (*serverconfigpb.GetServerConfigResponse, error) {
	resp := &serverconfigpb.GetServerConfigResponse{}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		resp.Error = "config name required"
		return resp, nil
	}

	cfg, ok, err := s.dm.GetServerConfig(name)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	if !ok {
		resp.Error = "config not found"
		return resp, nil
	}

	resp.Config = toProtoServerConfig(cfg)
	return resp, nil
}

func (s *ServerConfigService) SaveServerConfig(ctx context.Context, req *serverconfigpb.SaveServerConfigRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return &respb.StringResponse{Error: "config name required"}, nil
	}
	cfg := req.GetConfig()
	if cfg == nil {
		return &respb.StringResponse{Error: "config payload required"}, nil
	}

	if err := s.dm.SaveServerConfig(name, fromProtoServerConfig(cfg)); err != nil {
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("saved server config %s", name)}, nil
}

func (s *ServerConfigService) DeleteServerConfig(ctx context.Context, req *serverconfigpb.DeleteServerConfigRequest) (*respb.StringResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return &respb.StringResponse{Error: "config name required"}, nil
	}
	err := s.dm.DeleteServerConfig(name)
	if err != nil {
		if os.IsNotExist(err) {
			return &respb.StringResponse{Result: fmt.Sprintf("config %s not found", name)}, nil
		}
		return &respb.StringResponse{Error: err.Error()}, nil
	}
	return &respb.StringResponse{Result: fmt.Sprintf("deleted server config %s", name)}, nil
}

func toProtoServerConfig(cfg defines.ControlConfig) *serverconfigpb.ServerConfig {
	return &serverconfigpb.ServerConfig{
		AuthServerAddress:    cfg.AuthServerAddress,
		AuthServerToken:      cfg.AuthServerToken,
		AuthUsername:         cfg.AuthUsername,
		AuthPassword:         cfg.AuthPassword,
		RentalServerCode:     cfg.RentalServerCode,
		RentalServerPasscode: cfg.RentalServerPasscode,
	}
}

func fromProtoServerConfig(cfg *serverconfigpb.ServerConfig) defines.ControlConfig {
	return defines.ControlConfig{
		AuthServerAddress:    cfg.GetAuthServerAddress(),
		AuthServerToken:      cfg.GetAuthServerToken(),
		AuthUsername:         cfg.GetAuthUsername(),
		AuthPassword:         cfg.GetAuthPassword(),
		RentalServerCode:     cfg.GetRentalServerCode(),
		RentalServerPasscode: cfg.GetRentalServerPasscode(),
	}
}
