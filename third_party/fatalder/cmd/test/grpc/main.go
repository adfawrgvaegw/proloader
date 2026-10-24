package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	interruptpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/interrupt_config"
	serverconfigpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/server_config"
	taskspb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/tasks"
	taskscfgpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/define/tasks_config"
	framepb "github.com/Yeah114/Fatalder/cmd/grpc/pb/frame"
	reqpb "github.com/Yeah114/Fatalder/cmd/grpc/pb/request"
	respb "github.com/Yeah114/Fatalder/cmd/grpc/pb/response"
	"github.com/coder/websocket"
	"github.com/pterm/pterm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

var testLogger = pterm.DefaultLogger.WithTime(true).WithLevel(pterm.LogLevelInfo)

func main() {
	addr := flag.String("addr", "127.0.0.1:12465", "Fatalder gRPC server address. Use host:8080/grpc/12465 for proxy mode.")
	timeout := flag.Duration("timeout", 10*time.Second, "request timeout")
	mode := flag.String("mode", "frame", "Operation mode: frame, server-list, server-save, server-delete, interrupt-list, interrupt-save, interrupt-delete, tasks-list, tasks-save, tasks-delete")
	setKey := flag.String("set", "", "API Key to set when mode=frame")
	configName := flag.String("name", "", "Config name for server/interrupt/tasks operations")
	serverCode := flag.String("server-code", "", "Rental server code (server-save)")
	serverPass := flag.String("server-passcode", "", "Rental server passcode (server-save)")
	authAddr := flag.String("auth-addr", "", "Auth server address (server-save)")
	authToken := flag.String("auth-token", "", "Auth server token (server-save)")
	authUser := flag.String("auth-user", "", "Auth username (server-save)")
	authPass := flag.String("auth-pass", "", "Auth password (server-save)")
	interruptUUID := flag.String("interrupt-uuid", "", "Interrupt UUID (interrupt-save)")
	interruptData := flag.String("interrupt-data", "", "Interrupt data entries separated by comma (interrupt-save)")
	tasksJSON := flag.String("tasks-json", "", "JSON array of tasks (protojson format) for tasks-save")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	dialPlan, err := resolveDialPlan(strings.TrimSpace(*addr))
	if err != nil {
		testLogger.Fatal("地址解析失败", testLogger.Args("addr", *addr, "error", err))
	}

	transportCreds := insecure.NewCredentials()
	if dialPlan.tlsConfig != nil {
		transportCreds = credentials.NewTLS(dialPlan.tlsConfig)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(transportCreds),
	}

	if dialPlan.websocketURL != "" {
		dialOpts = append(dialOpts, grpc.WithContextDialer(newWebSocketDialer(dialPlan.websocketURL)))
	}

	conn, err := grpc.DialContext(
		ctx,
		dialPlan.address,
		dialOpts...,
	)
	if err != nil {
		testLogger.Fatal("连接 gRPC 服务器失败", testLogger.Args("addr", *addr, "error", err))
	}
	defer conn.Close()

	switch strings.ToLower(*mode) {
	case "frame":
		runFrameTest(ctx, conn, *setKey)
	case "server-list":
		runServerList(ctx, conn)
	case "server-save":
		runServerSave(ctx, conn, configName, serverCode, serverPass, authAddr, authToken, authUser, authPass)
	case "server-delete":
		runServerDelete(ctx, conn, configName)
	case "interrupt-list":
		runInterruptList(ctx, conn)
	case "interrupt-save":
		runInterruptSave(ctx, conn, configName, interruptUUID, interruptData)
	case "interrupt-delete":
		runInterruptDelete(ctx, conn, configName)
	case "tasks-list":
		runTasksList(ctx, conn)
	case "tasks-save":
		runTasksSave(ctx, conn, configName, tasksJSON)
	case "tasks-delete":
		runTasksDelete(ctx, conn, configName)
	default:
		testLogger.Fatal("不支持的模式", testLogger.Args("mode", *mode))
	}
}

func runFrameTest(ctx context.Context, conn *grpc.ClientConn, apiKey string) {
	client := framepb.NewFrameServiceClient(conn)
	fmt.Println(client.Ping(ctx, &reqpb.EmptyRequest{}))
	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" {
		resp, err := client.SetAPIKey(ctx, &reqpb.StringRequest{Data: apiKey})
		if err != nil {
			testLogger.Fatal("SetAPIKey 调用失败", testLogger.Args("error", err))
		}
		printStringResponse("SetAPIKey", resp)
	}

	resp, err := client.GetAPIKey(ctx, &reqpb.EmptyRequest{})
	if err != nil {
		testLogger.Fatal("GetAPIKey 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("GetAPIKey", resp)
}

func runServerList(ctx context.Context, conn *grpc.ClientConn) {
	client := serverconfigpb.NewServerConfigServiceClient(conn)
	resp, err := client.ListServerConfigs(ctx, &reqpb.EmptyRequest{})
	if err != nil {
		testLogger.Fatal("ListServerConfigs 调用失败", testLogger.Args("error", err))
	}
	if resp.GetError() != "" {
		fmt.Printf("ListServerConfigs error: %s\n", resp.GetError())
		return
	}
	if len(resp.GetConfigs()) == 0 {
		fmt.Println("No server configs found.")
		return
	}
	for _, cfg := range resp.GetConfigs() {
		fmt.Printf("Server config %s: %+v\n", cfg.GetName(), cfg.GetConfig())
	}
}

func runServerSave(ctx context.Context, conn *grpc.ClientConn, name, serverCode, serverPass, authAddr, authToken, authUser, authPass *string) {
	if strings.TrimSpace(*name) == "" {
		testLogger.Fatal("server-save requires --name")
	}
	if strings.TrimSpace(*serverCode) == "" {
		testLogger.Fatal("server-save requires --server-code")
	}
	client := serverconfigpb.NewServerConfigServiceClient(conn)
	req := &serverconfigpb.SaveServerConfigRequest{
		Name: strings.TrimSpace(*name),
		Config: &serverconfigpb.ServerConfig{
			AuthServerAddress:    strings.TrimSpace(*authAddr),
			AuthServerToken:      strings.TrimSpace(*authToken),
			AuthUsername:         strings.TrimSpace(*authUser),
			AuthPassword:         strings.TrimSpace(*authPass),
			RentalServerCode:     strings.TrimSpace(*serverCode),
			RentalServerPasscode: strings.TrimSpace(*serverPass),
		},
	}
	resp, err := client.SaveServerConfig(ctx, req)
	if err != nil {
		testLogger.Fatal("SaveServerConfig 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("SaveServerConfig", resp)
}

func runServerDelete(ctx context.Context, conn *grpc.ClientConn, name *string) {
	if strings.TrimSpace(*name) == "" {
		testLogger.Fatal("server-delete requires --name")
	}
	client := serverconfigpb.NewServerConfigServiceClient(conn)
	resp, err := client.DeleteServerConfig(ctx, &serverconfigpb.DeleteServerConfigRequest{Name: strings.TrimSpace(*name)})
	if err != nil {
		testLogger.Fatal("DeleteServerConfig 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("DeleteServerConfig", resp)
}

func runInterruptList(ctx context.Context, conn *grpc.ClientConn) {
	client := interruptpb.NewInterruptConfigServiceClient(conn)
	resp, err := client.ListInterruptConfigs(ctx, &reqpb.EmptyRequest{})
	if err != nil {
		testLogger.Fatal("ListInterruptConfigs 调用失败", testLogger.Args("error", err))
	}
	if resp.GetError() != "" {
		fmt.Printf("ListInterruptConfigs error: %s\n", resp.GetError())
		return
	}
	if len(resp.GetConfigs()) == 0 {
		fmt.Println("No interrupt configs found.")
		return
	}
	for _, cfg := range resp.GetConfigs() {
		fmt.Printf("Interrupt %s: uuid=%s data=%v\n", cfg.GetName(), cfg.GetConfig().GetUuid(), cfg.GetConfig().GetData())
	}
}

func runInterruptSave(ctx context.Context, conn *grpc.ClientConn, name, uuidFlag, dataFlag *string) {
	if strings.TrimSpace(*name) == "" {
		testLogger.Fatal("interrupt-save requires --name")
	}
	data := []string{}
	if trimmed := strings.TrimSpace(*dataFlag); trimmed != "" {
		for _, entry := range strings.Split(trimmed, ",") {
			entry = strings.TrimSpace(entry)
			if entry != "" {
				data = append(data, entry)
			}
		}
	}
	client := interruptpb.NewInterruptConfigServiceClient(conn)
	req := &interruptpb.SaveInterruptConfigRequest{
		Name: strings.TrimSpace(*name),
		Config: &interruptpb.InterruptConfig{
			Name: strings.TrimSpace(*name),
			Uuid: strings.TrimSpace(*uuidFlag),
			Data: data,
		},
	}
	resp, err := client.SaveInterruptConfig(ctx, req)
	if err != nil {
		testLogger.Fatal("SaveInterruptConfig 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("SaveInterruptConfig", resp)
}

func runInterruptDelete(ctx context.Context, conn *grpc.ClientConn, name *string) {
	if strings.TrimSpace(*name) == "" {
		testLogger.Fatal("interrupt-delete requires --name")
	}
	client := interruptpb.NewInterruptConfigServiceClient(conn)
	resp, err := client.DeleteInterruptConfig(ctx, &interruptpb.DeleteInterruptConfigRequest{Name: strings.TrimSpace(*name)})
	if err != nil {
		testLogger.Fatal("DeleteInterruptConfig 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("DeleteInterruptConfig", resp)
}

func runTasksList(ctx context.Context, conn *grpc.ClientConn) {
	client := taskscfgpb.NewTasksConfigServiceClient(conn)
	resp, err := client.ListTasksConfigs(ctx, &reqpb.EmptyRequest{})
	if err != nil {
		testLogger.Fatal("ListTasksConfigs 调用失败", testLogger.Args("error", err))
	}
	if resp.GetError() != "" {
		fmt.Printf("ListTasksConfigs error: %s\n", resp.GetError())
		return
	}
	if len(resp.GetConfigs()) == 0 {
		fmt.Println("No tasks configs found.")
		return
	}
	marshaler := protojson.MarshalOptions{Multiline: true, Indent: "  "}
	for _, entry := range resp.GetConfigs() {
		fmt.Printf("Tasks config %s:\n", entry.GetName())
		for i, t := range entry.GetTasks() {
			data, _ := marshaler.Marshal(t)
			fmt.Printf("  Task %d:\n%s\n", i+1, string(data))
		}
	}
}

func runTasksSave(ctx context.Context, conn *grpc.ClientConn, name, tasksJSON *string) {
	if strings.TrimSpace(*name) == "" {
		testLogger.Fatal("tasks-save requires --name")
	}
	tasks, err := parseTasksJSON(*tasksJSON)
	if err != nil {
		testLogger.Fatal("任务 JSON 无效", testLogger.Args("error", err))
	}
	client := taskscfgpb.NewTasksConfigServiceClient(conn)
	resp, err := client.SaveTasksConfig(ctx, &taskscfgpb.SaveTasksConfigRequest{
		Name:  strings.TrimSpace(*name),
		Tasks: tasks,
	})
	if err != nil {
		testLogger.Fatal("SaveTasksConfig 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("SaveTasksConfig", resp)
}

func runTasksDelete(ctx context.Context, conn *grpc.ClientConn, name *string) {
	if strings.TrimSpace(*name) == "" {
		testLogger.Fatal("tasks-delete requires --name")
	}
	client := taskscfgpb.NewTasksConfigServiceClient(conn)
	resp, err := client.DeleteTasksConfig(ctx, &taskscfgpb.DeleteTasksConfigRequest{Name: strings.TrimSpace(*name)})
	if err != nil {
		testLogger.Fatal("DeleteTasksConfig 调用失败", testLogger.Args("error", err))
	}
	printStringResponse("DeleteTasksConfig", resp)
}

func parseTasksJSON(raw string) ([]*taskspb.Task, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []*taskspb.Task{
			{
				Type: taskspb.TaskType_TASK_TYPE_COMMAND,
				Config: &taskspb.Task_Command{
					Command: &taskspb.CommandTaskConfig{
						Command:  "/say hello",
						Identity: "player",
					},
				},
			},
		}, nil
	}

	var entries []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}

	res := make([]*taskspb.Task, 0, len(entries))
	for _, entry := range entries {
		task := &taskspb.Task{}
		if err := protojson.Unmarshal(entry, task); err != nil {
			return nil, err
		}
		res = append(res, task)
	}
	return res, nil
}

func printStringResponse(prefix string, resp *respb.StringResponse) {
	if resp.GetError() != "" {
		fmt.Printf("%s error: %s\n", prefix, resp.GetError())
		return
	}
	fmt.Printf("%s success: %s\n", prefix, resp.GetResult())
}

type dialPlan struct {
	address      string
	websocketURL string
	tlsConfig    *tls.Config
}

func resolveDialPlan(raw string) (*dialPlan, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("地址为空")
	}

	const marker = "/grpc/"
	if idx := strings.Index(raw, marker); idx >= 0 {
		base := strings.TrimSpace(raw[:idx])
		if base == "" {
			return nil, fmt.Errorf("代理地址缺失")
		}
		service := raw[idx+len(marker):]
		if service == "" {
			return nil, fmt.Errorf("缺少服务名")
		}
		if slash := strings.IndexByte(service, '/'); slash >= 0 {
			service = service[:slash]
		}
		service = strings.TrimSpace(service)
		if service == "" {
			return nil, fmt.Errorf("缺少服务名")
		}
		return buildWebSocketDialPlan(base, service)
	}

	return buildDirectDialPlan(raw)
}

func buildWebSocketDialPlan(base, service string) (*dialPlan, error) {
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("解析代理地址失败: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("代理地址缺少主机名")
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}

	u.Path = strings.TrimSuffix(u.Path, "/") + "/grpc/" + url.PathEscape(service)

	host := u.Hostname()
	port := u.Port()
	if host == "" {
		return nil, fmt.Errorf("代理地址缺少主机名")
	}
	if port == "" {
		if u.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return &dialPlan{
		address:      net.JoinHostPort(host, port),
		websocketURL: u.String(),
	}, nil
}

func buildDirectDialPlan(raw string) (*dialPlan, error) {
	var host, port, scheme string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("解析地址失败: %w", err)
		}
		if u.Host == "" {
			return nil, fmt.Errorf("地址缺少主机名")
		}
		host = u.Hostname()
		port = u.Port()
		scheme = strings.ToLower(u.Scheme)
	} else {
		h, p, err := net.SplitHostPort(raw)
		if err != nil {
			host = raw
		} else {
			host = h
			port = p
		}
	}
	if host == "" {
		return nil, fmt.Errorf("地址缺少主机名")
	}
	if port == "" {
		if scheme == "https" || scheme == "grpcs" {
			port = "443"
		} else {
			port = "12465"
		}
	}

	plan := &dialPlan{
		address: net.JoinHostPort(host, port),
	}
	if scheme == "https" || scheme == "grpcs" {
		plan.tlsConfig = &tls.Config{
			ServerName: host,
		}
	}
	return plan, nil
}

func newWebSocketDialer(wsURL string) func(ctx context.Context, target string) (net.Conn, error) {
	remote := wsURL
	if u, err := url.Parse(wsURL); err == nil && u.Host != "" {
		remote = u.Host
	}
	return func(ctx context.Context, target string) (net.Conn, error) {
		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			return nil, err
		}
		return &wsClientConn{
			conn:   conn,
			remote: remote,
		}, nil
	}
}

type wsClientConn struct {
	conn    *websocket.Conn
	remote  string
	readBuf []byte
}

func (c *wsClientConn) Read(b []byte) (int, error) {
	if len(c.readBuf) == 0 {
		for {
			msgType, data, err := c.conn.Read(context.Background())
			if err != nil {
				return 0, err
			}
			if msgType != websocket.MessageBinary {
				continue
			}
			c.readBuf = data
			break
		}
	}

	n := copy(b, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

func (c *wsClientConn) Write(b []byte) (int, error) {
	if err := c.conn.Write(context.Background(), websocket.MessageBinary, append([]byte(nil), b...)); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *wsClientConn) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "client closed")
}

func (c *wsClientConn) LocalAddr() net.Addr {
	return wsAddr("local")
}

func (c *wsClientConn) RemoteAddr() net.Addr {
	return wsAddr(c.remote)
}

func (c *wsClientConn) SetDeadline(time.Time) error {
	return nil
}

func (c *wsClientConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *wsClientConn) SetWriteDeadline(time.Time) error {
	return nil
}

type wsAddr string

func (a wsAddr) Network() string {
	return "websocket"
}

func (a wsAddr) String() string {
	return string(a)
}
