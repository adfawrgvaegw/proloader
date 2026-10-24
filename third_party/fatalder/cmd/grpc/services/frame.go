package services

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"sync"
	"time"

	framepb "github.com/Yeah114/Fatalder/cmd/grpc/pb/frame"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	"github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/Fatalder/utils/user_center"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type FrameService struct {
	framepb.UnimplementedFrameServiceServer
	password        string
	verifiedClients sync.Map
}

func NewFrameService(password string) *FrameService {
	return &FrameService{
		password: strings.TrimSpace(password),
	}
}

func (s *FrameService) Ping(ctx context.Context, _ *reqpb.EmptyRequest) (*respb.Int64Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &respb.Int64Response{
		Result: time.Now().UnixMilli(),
	}, nil
}

func (s *FrameService) HasPassword(ctx context.Context, _ *reqpb.EmptyRequest) (*respb.BoolResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &respb.BoolResponse{
		Result: s.password != "",
	}, nil
}

func (s *FrameService) VerifyPassword(ctx context.Context, req *reqpb.StringRequest) (*respb.BoolResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if s.password == "" {
		s.markPeerVerified(ctx, true)
		return &respb.BoolResponse{Result: true}, nil
	}

	input := strings.TrimSpace(req.GetData())
	if input == "" {
		s.markPeerVerified(ctx, false)
		return &respb.BoolResponse{Error: "密码不能为空"}, nil
	}

	match := subtle.ConstantTimeCompare([]byte(input), []byte(s.password)) == 1
	if !match {
		s.markPeerVerified(ctx, false)
		return &respb.BoolResponse{Result: false, Error: "密码错误"}, nil
	}

	s.markPeerVerified(ctx, true)
	return &respb.BoolResponse{Result: true}, nil
}

func (s *FrameService) GetAPIKey(ctx context.Context, _ *reqpb.EmptyRequest) (*respb.StringResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg := utils.GetConfig().GetUserCenterConfig()
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return &respb.StringResponse{
			Error: "API Key 未配置",
		}, nil
	}

	return &respb.StringResponse{
		Result: apiKey,
	}, nil
}

func (s *FrameService) SetAPIKey(ctx context.Context, req *reqpb.StringRequest) (*respb.StringResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	apiKey := strings.TrimSpace(req.GetData())
	if apiKey == "" {
		return &respb.StringResponse{
			Error: "API Key 不能为空",
		}, nil
	}

	config := utils.GetConfig()
	ucCfg := config.GetUserCenterConfig()
	ucCfg.APIKey = apiKey
	if err := config.SetUserCenterConfig(ucCfg); err != nil {
		return &respb.StringResponse{
			Error: fmt.Sprintf("保存配置失败: %v", err),
		}, nil
	}

	if err := reinitUserCenterOperator(strings.TrimSpace(ucCfg.BaseRoot), apiKey); err != nil {
		return &respb.StringResponse{
			Error: fmt.Sprintf("初始化用户中心失败: %v", err),
		}, nil
	}

	return &respb.StringResponse{
		Result: "API Key 已更新",
	}, nil
}

func reinitUserCenterOperator(baseRoot, apiKey string) error {
	if baseRoot == "" || apiKey == "" {
		user_center.SetGlobalOperator(nil)
		if baseRoot == "" {
			return nil
		}
		return fmt.Errorf("API Key 未配置")
	}
	op, err := user_center.NewUserCenterOperater(baseRoot, apiKey)
	if err != nil {
		return err
	}
	user_center.SetGlobalOperator(op)
	return nil
}

func (s *FrameService) markPeerVerified(ctx context.Context, ok bool) {
	addr := peerAddrFromContext(ctx)
	if addr == "" {
		return
	}
	if ok {
		s.verifiedClients.Store(addr, struct{}{})
		return
	}
	s.verifiedClients.Delete(addr)
}

func (s *FrameService) isPeerVerified(ctx context.Context) bool {
	if s.password == "" {
		return true
	}
	addr := peerAddrFromContext(ctx)
	if addr == "" {
		return false
	}
	_, ok := s.verifiedClients.Load(addr)
	return ok
}

func peerAddrFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	p, ok := peer.FromContext(ctx)
	if !ok || p == nil || p.Addr == nil {
		return ""
	}
	return p.Addr.String()
}

var frameAuthBypass = map[string]struct{}{
	framepb.FrameService_Ping_FullMethodName:           {},
	framepb.FrameService_HasPassword_FullMethodName:    {},
	framepb.FrameService_VerifyPassword_FullMethodName: {},
}

func NewFrameAuthUnaryInterceptor(fs *FrameService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if _, ok := frameAuthBypass[info.FullMethod]; ok {
			return handler(ctx, req)
		}
		if fs.isPeerVerified(ctx) {
			return handler(ctx, req)
		}

		fs.markPeerVerified(ctx, false)
		return nil, status.Error(codes.PermissionDenied, "尚未通过框架密码验证")
	}
}
