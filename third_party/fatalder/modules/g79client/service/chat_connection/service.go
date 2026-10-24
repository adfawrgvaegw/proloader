package chat_connection

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	g79client "github.com/Yeah114/g79client"
)

// ServerEndpoint 描述一次具体连接所需的主机与端口。
type ServerEndpoint struct {
	Host  string
	Port  int
	Entry g79client.G79ChatServerEntry
}

// ChatConnectionService 负责全局聊天服务器列表的获取与连接管理。
type ChatConnectionService struct {
	client *g79client.Client

	mu      sync.RWMutex
	servers []g79client.G79ChatServerEntry

	randMu sync.Mutex
	rnd    *rand.Rand
}

// ispHostFallback 不同 ISP 对应的域名字段。
var ispHostFallback = map[int]func(g79client.G79ChatServerEntry) string{
	10000: func(entry g79client.G79ChatServerEntry) string { return entry.CTCCHost },
	10010: func(entry g79client.G79ChatServerEntry) string { return entry.CUCCHost },
	10086: func(entry g79client.G79ChatServerEntry) string { return entry.CMCCHost },
	0:     func(entry g79client.G79ChatServerEntry) string { return entry.IP },
}

// NewChatConnectionService 根据 G79 客户端实例构建聊天连接服务。
func NewChatConnectionService(client *g79client.Client) (*ChatConnectionService, error) {
	if client == nil {
		return nil, errors.New("chat_connection.NewChatConnectionService: client 不能为空")
	}

	svc := &ChatConnectionService{
		client: client,
		rnd:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := svc.primeServers(); err != nil {
		// 允许延迟错误，调用方可稍后再刷新
		return svc, err
	}

	return svc, nil
}

func (s *ChatConnectionService) primeServers() error {
	list, err := g79client.GetGlobalG79ChatServers()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.servers = cloneChatServers(list)
	s.mu.Unlock()
	return nil
}

// Client 返回底层 G79 客户端。
func (s *ChatConnectionService) Client() *g79client.Client {
	return s.client
}

// Servers 返回当前缓存的聊天服务器列表。
func (s *ChatConnectionService) Servers() ([]g79client.G79ChatServerEntry, error) {
	s.mu.RLock()
	if s.servers != nil {
		list := cloneChatServers(s.servers)
		s.mu.RUnlock()
		return list, nil
	}
	s.mu.RUnlock()
	return s.RefreshServers()
}

// RefreshServers 强制刷新聊天服务器列表。
func (s *ChatConnectionService) RefreshServers() ([]g79client.G79ChatServerEntry, error) {
	list, err := g79client.RefreshG79ChatServers()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.servers = cloneChatServers(list)
	s.mu.Unlock()
	return cloneChatServers(list), nil
}

// SelectServer 根据 ISP ID 随机选择一个聊天服务器。
func (s *ChatConnectionService) SelectServer(ispID int) (*ServerEndpoint, error) {
	servers, err := s.Servers()
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, errors.New("chat_connection.SelectServer: 当前没有可用的聊天服务器")
	}

	entry := s.pickRandom(servers)
	host := resolveHostByISP(entry, ispID)
	if host == "" {
		return nil, errors.New("chat_connection.SelectServer: 找不到有效域名")
	}

	return &ServerEndpoint{
		Host:  host,
		Port:  entry.Port,
		Entry: entry,
	}, nil
}

// Dial 选择合适的聊天服务器并建立连接。
func (s *ChatConnectionService) Dial(ctx context.Context, ispID int) (*ChatConnection, error) {
	endpoint, err := s.SelectServer(ispID)
	if err != nil {
		return nil, err
	}
	return s.DialWithEndpoint(ctx, endpoint)
}

// DialWithEndpoint 使用指定端点建立聊天连接。
func (s *ChatConnectionService) DialWithEndpoint(ctx context.Context, endpoint *ServerEndpoint) (*ChatConnection, error) {
	if endpoint == nil {
		return nil, errors.New("chat_connection.DialWithEndpoint: endpoint 不能为空")
	}

	d := &net.Dialer{}
	if deadline, ok := ctx.Deadline(); ok {
		d.Timeout = time.Until(deadline)
	}

	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)))
	if err != nil {
		return nil, err
	}

	chatConn := newChatConnection(s, conn, endpoint.Entry)
	if err := chatConn.handshake(ctx); err != nil {
		conn.Close()
		return nil, err
	}

	chatConn.startLoops()
	return chatConn, nil
}

func (s *ChatConnectionService) pickRandom(servers []g79client.G79ChatServerEntry) g79client.G79ChatServerEntry {
	if len(servers) == 1 {
		return servers[0]
	}
	s.randMu.Lock()
	idx := s.rnd.Intn(len(servers))
	s.randMu.Unlock()
	return servers[idx]
}

func resolveHostByISP(entry g79client.G79ChatServerEntry, ispID int) string {
	if entry.IspEnabled.Bool() {
		if getter, ok := ispHostFallback[ispID]; ok {
			if host := getter(entry); host != "" {
				return host
			}
		}
	}

	if entry.IP != "" {
		return entry.IP
	}
	if entry.CTCCHost != "" {
		return entry.CTCCHost
	}
	if entry.CMCCHost != "" {
		return entry.CMCCHost
	}
	if entry.CUCCHost != "" {
		return entry.CUCCHost
	}
	return ""
}

func cloneChatServers(src []g79client.G79ChatServerEntry) []g79client.G79ChatServerEntry {
	if len(src) == 0 {
		return nil
	}
	dst := make([]g79client.G79ChatServerEntry, len(src))
	copy(dst, src)
	return dst
}
