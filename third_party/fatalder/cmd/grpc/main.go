package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/pterm/pterm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"

	"github.com/Yeah114/Fatalder/cmd/args"
	cmd_utils "github.com/Yeah114/Fatalder/cmd/utils"
	"github.com/Yeah114/Fatalder/cmd/common"
	"github.com/Yeah114/Fatalder/utils"
	framepb "github.com/Yeah114/Fatalder/cmd/grpc/pb/frame"
	"github.com/Yeah114/Fatalder/cmd/grpc/services"
	"github.com/Yeah114/Fatalder/utils/data_manager"
	"github.com/Yeah114/Fatalder/utils/user_center"
)

var grpcLogger = pterm.DefaultLogger.
	WithLevel(pterm.LogLevelInfo).
	WithTime(true).
	WithMaxWidth(160)

func main() {
	common.CheckAndShowNewCommits()

	cmd_utils.RegistTemporaryAPIKeyFlag()

	args.Flags().StringP(
		"host",
		"H",
		"0.0.0.0",
		"gRPC server host",
	)

	args.Flags().IntP(
		"port",
		"P",
		12465,
		"gRPC server prot",
	)

	args.Flags().StringP(
		"password",
		"p",
		"",
		"gRPC server password",
	)

	args.Flags().StringP(
		"multiplex",
		"m",
		"http://0.0.0.0:17085",
		"multiplex server url",
	)

	args.Flags().StringP(
		"name",
		"n",
		"",
		"multiplex service name",
	)

	args.Flags().BoolP(
		"debug",
		"d",
		false,
		"Enable verbose gRPC call logging",
	)

	// args 包会自动初始化所有系统
	err, shouldContinue := args.Parse()
	if err != nil {
		panic(err)
	}
	if !shouldContinue {
		return
	}

	// 显示程序信息
	common.ShowProgramInfo()

	cliAPIKey := cmd_utils.GetTemporaryAPIKey()
	if cliAPIKey != "" {
		cfg := utils.GetConfig().GetUserCenterConfig()
		base := strings.TrimSpace(cfg.BaseRoot)
		if base == "" {
			base = "https://fatalder.yeah114.top"
		}
		op, err := user_center.NewUserCenterOperater(base, cliAPIKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化用户中心加密客户端失败: %v\n", err)
		} else {
			user_center.SetGlobalOperator(op)
			fmt.Fprintf(os.Stdout, "已使用临时 API Key（本次运行有效，不写入配置）。\n")
		}
	} else {
		if err := user_center.InitGlobalFromConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "初始化用户中心加密客户端失败: %v\n", err)
		}
	}

	if err := user_center.InitGlobalFromConfig(); err != nil {
		grpcLogger.Warn("初始化用户中心加密客户端失败", grpcLogger.Args("error", err))
	}

	host, _ := args.Flags().GetString("host")
	port, _ := args.Flags().GetInt("port")
	password, _ := args.Flags().GetString("password")
	debugCalls, _ := args.Flags().GetBool("debug")
	multiplexURL, _ := args.Flags().GetString("multiplex")
	serviceName, _ := args.Flags().GetString("name")
	if debugCalls {
		grpcLogger = grpcLogger.WithLevel(pterm.LogLevelDebug)
	}
	if port == 0 {
		port, err = getAvailablePort(host)
		if err != nil {
			grpcLogger.Fatal("无法获取随机端口", grpcLogger.Args("error", err))
		}
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		grpcLogger.Fatal("无法监听地址", grpcLogger.Args("addr", addr, "error", err))
	}

	frameSvc := services.NewFrameService(password)
	server := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				newDebugUnaryInterceptor(debugCalls),
				services.NewFrameAuthUnaryInterceptor(frameSvc),
			),
		),
		grpc.StatsHandler(&loggingStatsHandler{}),
	)
	framepb.RegisterFrameServiceServer(server, frameSvc)
	services.RegisterConfigServices(server, data_manager.GlobalDataManager)
	services.RegisterTaskGroupManagerService(server)
	reflection.Register(server)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		grpcLogger.Warn("捕获到终止信号，准备优雅关闭 Fatalder gRPC Server ...")
		server.GracefulStop()
	}()

	startMultiplexClient(strings.TrimSpace(multiplexURL), strings.TrimSpace(serviceName), addr)

	grpcLogger.Info("Fatalder gRPC 服务已启动", grpcLogger.Args("listen", addr))
	if err := server.Serve(lis); err != nil {
		grpcLogger.Fatal("gRPC 服务异常退出", grpcLogger.Args("error", err))
	}
}

func getAvailablePort(host string) (int, error) {
	address, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:0", host))
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", address)
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func newDebugUnaryInterceptor(enabled bool) grpc.UnaryServerInterceptor {
	if !enabled {
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		peerAddr := "unknown"
		if p, ok := peer.FromContext(ctx); ok && p != nil && p.Addr != nil {
			peerAddr = p.Addr.String()
		}
		grpcLogger.Debug("gRPC 调用", grpcLogger.Args("method", info.FullMethod, "peer", peerAddr))
		return handler(ctx, req)
	}
}

type loggingStatsHandler struct{}

func (h *loggingStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	remote := "unknown"
	if info != nil && info.RemoteAddr != nil {
		remote = info.RemoteAddr.String()
	}
	grpcLogger.Info("客户端连接建立", grpcLogger.Args("remote", remote))
	return ctx
}

func (h *loggingStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

func (h *loggingStatsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *loggingStatsHandler) HandleRPC(context.Context, stats.RPCStats) {}

func startMultiplexClient(baseURL, serviceName, backendAddr string) {
	baseURL = strings.TrimSpace(baseURL)
	serviceName = strings.TrimSpace(serviceName)
	if baseURL == "" || serviceName == "" {
		return
	}

	backendAddr = normalizeBackendAddr(backendAddr)

	go func() {
		grpcLogger.Info("Multiplex 客户端已启用", grpcLogger.Args("url", baseURL, "name", serviceName, "backend", backendAddr))
		for {
			if err := runMultiplexControl(baseURL, serviceName, backendAddr); err != nil {
				grpcLogger.Warn("Multiplex 控制连接结束", grpcLogger.Args("error", err))
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

func normalizeBackendAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	h := strings.TrimSpace(host)
	if h == "" || h == "0.0.0.0" || h == "::" {
		h = "127.0.0.1"
	}
	return net.JoinHostPort(h, port)
}

func runMultiplexControl(baseURL, serviceName, backendAddr string) error {
	if err := doMultiplexRegister(baseURL, serviceName); err != nil {
		return err
	}

	controlURL, err := buildWebSocketURL(baseURL, "/handle/"+url.PathEscape(serviceName))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	conn, _, err := websocket.Dial(ctx, controlURL, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("连接控制 WebSocket 失败: %w", err)
	}
	defer conn.Close(websocket.StatusGoingAway, "control closed")

	return handleControlMessages(conn, baseURL, serviceName, backendAddr)
}

type multiplexTask struct {
	Type    string `json:"type"`
	Session string `json:"session"`
	Path    string `json:"path"`
}

func doMultiplexRegister(baseURL, serviceName string) error {
	registerURL, err := buildHTTPURL(baseURL, "/register/"+url.PathEscape(serviceName))
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, registerURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("注册 Multiplex 失败: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("注册 Multiplex 状态异常: %s", resp.Status)
	}
	return nil
}

func buildHTTPURL(baseURL, path string) (string, error) {
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" {
		base.Scheme = "http"
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	final := base.ResolveReference(ref)
	return final.String(), nil
}

func buildWebSocketURL(baseURL, path string) (string, error) {
	if strings.HasPrefix(path, "ws://") || strings.HasPrefix(path, "wss://") {
		return path, nil
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(base.Scheme) {
	case "", "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("不支持的 Multiplex scheme: %s", base.Scheme)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	final := base.ResolveReference(ref)
	return final.String(), nil
}

func handleControlMessages(conn *websocket.Conn, baseURL, serviceName, backendAddr string) error {
	for {
		msgType, data, err := conn.Read(context.Background())
		if err != nil {
			return err
		}
		if msgType != websocket.MessageText {
			continue
		}

		var task multiplexTask
		if err := json.Unmarshal(data, &task); err != nil {
			grpcLogger.Warn("无法解析 Multiplex 指令", grpcLogger.Args("error", err))
			continue
		}

		switch task.Type {
		case "start_tunnel":
			go func(t multiplexTask) {
				if err := startServerSession(baseURL, serviceName, t, backendAddr); err != nil {
					grpcLogger.Warn("处理 Multiplex 会话失败", grpcLogger.Args("session", t.Session, "error", err))
				}
			}(task)
		default:
			grpcLogger.Debug("忽略未知 Multiplex 指令", grpcLogger.Args("type", task.Type))
		}
	}
}

func startServerSession(baseURL, serviceName string, task multiplexTask, backendAddr string) error {
	if task.Session == "" {
		return errors.New("缺少 session 字段")
	}
	path := task.Path
	if path == "" {
		path = fmt.Sprintf("/server/%s?session=%s", url.PathEscape(serviceName), url.QueryEscape(task.Session))
	}
	serverURL, err := buildWebSocketURL(baseURL, path)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	wsConn, _, err := websocket.Dial(ctx, serverURL, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("连接 Multiplex 数据通道失败: %w", err)
	}

	backendConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		wsConn.Close(websocket.StatusInternalError, "backend unavailable")
		return fmt.Errorf("连接本地 gRPC 失败: %w", err)
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- pumpBackendToWebsocket(wsConn, backendConn)
	}()
	go func() {
		errCh <- pumpWebsocketToBackend(wsConn, backendConn)
	}()

	err = <-errCh
	backendConn.Close()
	wsConn.Close(websocket.StatusNormalClosure, "session closed")
	<-errCh
	return err
}

func pumpBackendToWebsocket(ws *websocket.Conn, backend net.Conn) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := backend.Read(buf)
		if n > 0 {
			if writeErr := ws.Write(context.Background(), websocket.MessageBinary, buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
	}
}

func pumpWebsocketToBackend(ws *websocket.Conn, backend net.Conn) error {
	for {
		msgType, data, err := ws.Read(context.Background())
		if err != nil {
			return err
		}
		if msgType != websocket.MessageBinary {
			continue
		}
		if len(data) == 0 {
			continue
		}
		if _, err := backend.Write(data); err != nil {
			return err
		}
	}
}
