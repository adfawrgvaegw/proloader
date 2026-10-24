package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const (
	defaultListenAddr = ":17085"
	sessionWait       = 10 * time.Second
)

var (
	listenAddr = flag.String("listen", defaultListenAddr, "Multiplex 服务监听地址，例如 0.0.0.0:17085")

	wsAcceptOpts = &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	}

	registry = newServerRegistry()
)

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/register/", registerServerHandler)
	mux.HandleFunc("/handle/", handleControlHandler)
	mux.HandleFunc("/grpc/", grpcTunnelHandler)
	mux.HandleFunc("/server/", serverTunnelHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("Multiplex WebSocket 代理启动，监听 %s", *listenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("代理服务异常退出: %v", err)
	}
}

func registerServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}
	name, err := parseNameFromPath("/register/", r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	registry.ensureServer(name)
	w.WriteHeader(http.StatusOK)
}

func handleControlHandler(w http.ResponseWriter, r *http.Request) {
	name, err := parseNameFromPath("/handle/", r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn, err := websocket.Accept(w, r, wsAcceptOpts)
	if err != nil {
		log.Printf("Handle WebSocket 握手失败: %v", err)
		return
	}
	log.Printf("服务器 %s 控制连接已建立", name)
	registry.attachControl(name, conn)
}

func grpcTunnelHandler(w http.ResponseWriter, r *http.Request) {
	name, err := parseNameFromPath("/grpc/", r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	clientConn, err := websocket.Accept(w, r, wsAcceptOpts)
	if err != nil {
		log.Printf("客户端 WebSocket 握手失败: %v", err)
		return
	}

	server := registry.getServer(name)
	if server == nil {
		_ = clientConn.Close(websocket.StatusPolicyViolation, "server not available")
		return
	}

	session := registry.createSession(name, clientConn)
	defer registry.removeSession(session.id)

	startMsg := controlMessage{
		Type:    "start_tunnel",
		Session: session.id,
		Path:    fmt.Sprintf("/server/%s?session=%s", url.PathEscape(name), url.QueryEscape(session.id)),
	}
	if err := server.send(startMsg); err != nil {
		log.Printf("通知服务器 %s 建立隧道失败: %v", name, err)
		_ = clientConn.Close(websocket.StatusInternalError, "server unavailable")
		return
	}

	serverConn, err := session.waitForServer(sessionWait)
	if err != nil {
		log.Printf("等待服务器 %s 建立数据连接超时: %v", name, err)
		_ = clientConn.Close(websocket.StatusPolicyViolation, "server timeout")
		return
	}

	log.Printf("隧道 %s/%s 已就绪", name, session.id)
	relayWebSockets(session.id, clientConn, serverConn)
}

func serverTunnelHandler(w http.ResponseWriter, r *http.Request) {
	name, err := parseNameFromPath("/server/", r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "缺少 session 参数", http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(w, r, wsAcceptOpts)
	if err != nil {
		log.Printf("服务器数据 WebSocket 握手失败: %v", err)
		return
	}

	if err := registry.attachServerConn(name, sessionID, conn); err != nil {
		log.Printf("附加服务器数据连接失败: %v", err)
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
}

type controlMessage struct {
	Type    string `json:"type"`
	Session string `json:"session"`
	Path    string `json:"path,omitempty"`
}

type serverRegistry struct {
	mu       sync.Mutex
	servers  map[string]*serverEntry
	sessions map[string]*sessionEntry
}

type serverEntry struct {
	name       string
	control    *websocket.Conn
	controlMu  sync.Mutex
	cancelFunc context.CancelFunc
}

type sessionEntry struct {
	id        string
	name      string
	client    *websocket.Conn
	serverCh  chan *websocket.Conn
	createdAt time.Time
}

func newServerRegistry() *serverRegistry {
	return &serverRegistry{
		servers:  make(map[string]*serverEntry),
		sessions: make(map[string]*sessionEntry),
	}
}

func (r *serverRegistry) ensureServer(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.servers[name]; !ok {
		r.servers[name] = &serverEntry{name: name}
	}
}

func (r *serverRegistry) getServer(name string) *serverEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	srv := r.servers[name]
	if srv == nil || srv.control == nil {
		return nil
	}
	return srv
}

func (r *serverRegistry) attachControl(name string, conn *websocket.Conn) {
	r.mu.Lock()
	entry, ok := r.servers[name]
	if !ok {
		entry = &serverEntry{name: name}
		r.servers[name] = entry
	}
	oldCancel := entry.cancelFunc
	entry.control = conn
	ctx, cancel := context.WithCancel(context.Background())
	entry.cancelFunc = cancel
	r.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}

	go r.monitorControl(ctx, name, conn)
}

func (r *serverRegistry) monitorControl(ctx context.Context, name string, conn *websocket.Conn) {
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "control closed")
		r.mu.Lock()
		if entry, ok := r.servers[name]; ok {
			entry.control = nil
			entry.cancelFunc = nil
		}
		r.mu.Unlock()
		log.Printf("服务器 %s 控制连接断开", name)
	}()

	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (s *serverEntry) send(msg controlMessage) error {
	if s == nil || s.control == nil {
		return errors.New("server not available")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	return s.control.Write(context.Background(), websocket.MessageText, data)
}

func (r *serverRegistry) createSession(name string, client *websocket.Conn) *sessionEntry {
	session := &sessionEntry{
		id:        uuid.NewString(),
		name:      name,
		client:    client,
		serverCh:  make(chan *websocket.Conn, 1),
		createdAt: time.Now(),
	}
	r.mu.Lock()
	r.sessions[session.id] = session
	r.mu.Unlock()
	return session
}

func (r *serverRegistry) attachServerConn(name, sessionID string, conn *websocket.Conn) error {
	r.mu.Lock()
	session, ok := r.sessions[sessionID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s 不存在", sessionID)
	}
	if session.name != name {
		return fmt.Errorf("session %s 不属于 %s", sessionID, name)
	}
	select {
	case session.serverCh <- conn:
		return nil
	default:
		return fmt.Errorf("session %s 已附加", sessionID)
	}
}

func (r *serverRegistry) removeSession(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

func (s *sessionEntry) waitForServer(timeout time.Duration) (*websocket.Conn, error) {
	select {
	case conn := <-s.serverCh:
		return conn, nil
	case <-time.After(timeout):
		return nil, errors.New("server timeout")
	}
}

func relayWebSockets(sessionID string, clientConn, serverConn *websocket.Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := proxyWebSocket(ctx, clientConn, serverConn); err != nil {
			cancel()
		}
	}()
	go func() {
		if err := proxyWebSocket(ctx, serverConn, clientConn); err != nil {
			cancel()
		}
	}()

	<-ctx.Done()
	clientConn.Close(websocket.StatusNormalClosure, "session closed")
	serverConn.Close(websocket.StatusNormalClosure, "session closed")
	log.Printf("隧道 %s 已关闭", sessionID)
}

func proxyWebSocket(ctx context.Context, src, dst *websocket.Conn) error {
	for {
		msgType, data, err := src.Read(ctx)
		if err != nil {
			return err
		}
		if msgType != websocket.MessageBinary {
			continue
		}
		if err := dst.Write(ctx, websocket.MessageBinary, data); err != nil {
			return err
		}
	}
}

func parseNameFromPath(prefix, path string) (string, error) {
	if !strings.HasPrefix(path, prefix) {
		return "", fmt.Errorf("路径错误: %s", path)
	}
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", fmt.Errorf("路径缺少名称: %s", path)
	}
	if strings.Contains(trimmed, "/") {
		return "", fmt.Errorf("路径不能包含额外的 '/': %s", path)
	}
	return trimmed, nil
}
