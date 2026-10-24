package link_connection

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

// LinkEndpoint 描述一个可用的 Link 服务器节点。
type LinkEndpoint struct {
	Entry g79client.G79LinkServerEntry
}

// LinkConnectionService 管理 Link 服务器列表与连接创建。
type LinkConnectionService struct {
	client *g79client.Client

	mu      sync.RWMutex
	servers []g79client.G79LinkServerEntry

	randMu sync.Mutex
	rnd    *rand.Rand
}

// NewLinkConnectionService 根据客户端信息创建服务。
func NewLinkConnectionService(client *g79client.Client) (*LinkConnectionService, error) {
	if client == nil {
		return nil, errors.New("link_connection.NewLinkConnectionService: client 不能为空")
	}

	svc := &LinkConnectionService{
		client: client,
		rnd:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := svc.primeServers(); err != nil {
		// 允许延迟错误返回给调用方
		return svc, err
	}
	return svc, nil
}

func (s *LinkConnectionService) primeServers() error {
	list, err := g79client.GetGlobalG79LinkServers()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.servers = cloneLinkServers(list)
	s.mu.Unlock()
	return nil
}

// Client 返回底层 G79 客户端。
func (s *LinkConnectionService) Client() *g79client.Client {
	return s.client
}

// Servers 返回缓存的服务器列表，如果缓存为空则尝试刷新。
func (s *LinkConnectionService) Servers() ([]g79client.G79LinkServerEntry, error) {
	s.mu.RLock()
	if s.servers != nil {
		list := cloneLinkServers(s.servers)
		s.mu.RUnlock()
		return list, nil
	}
	s.mu.RUnlock()
	return s.RefreshServers()
}

// RefreshServers 强制刷新服务器列表缓存。
func (s *LinkConnectionService) RefreshServers() ([]g79client.G79LinkServerEntry, error) {
	list, err := g79client.RefreshG79LinkServers()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.servers = cloneLinkServers(list)
	s.mu.Unlock()
	return cloneLinkServers(list), nil
}

// SelectEndpoint 随机选择一个可用节点。
func (s *LinkConnectionService) SelectEndpoint() (*LinkEndpoint, error) {
	servers, err := s.Servers()
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, errors.New("link_connection.SelectEndpoint: 当前没有可用的 link 服务器")
	}
	entry := s.pickRandom(servers)
	return &LinkEndpoint{Entry: entry}, nil
}

// Dial 选择节点并建立连接，等待登录完成。
func (s *LinkConnectionService) Dial(ctx context.Context) (*LinkConnection, error) {
	endpoint, err := s.SelectEndpoint()
	if err != nil {
		return nil, err
	}
	return s.DialEndpoint(ctx, endpoint)
}

// DialEndpoint 使用指定节点建立连接。
func (s *LinkConnectionService) DialEndpoint(ctx context.Context, endpoint *LinkEndpoint) (*LinkConnection, error) {
	if endpoint == nil {
		return nil, errors.New("link_connection.DialEndpoint: endpoint 不能为空")
	}

	d := &net.Dialer{}
	if deadline, ok := ctx.Deadline(); ok {
		d.Timeout = time.Until(deadline)
	}

	port := int(endpoint.Entry.Port.Int64())
	if port == 0 {
		return nil, errors.New("link_connection.DialEndpoint: 节点端口无效")
	}
	addr := net.JoinHostPort(endpoint.Entry.IP, strconv.Itoa(port))
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	linkConn, err := newLinkConnection(s, conn, endpoint.Entry)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if err := linkConn.waitForLogin(ctx); err != nil {
		linkConn.Close()
		return nil, err
	}
	return linkConn, nil
}

func (s *LinkConnectionService) pickRandom(servers []g79client.G79LinkServerEntry) g79client.G79LinkServerEntry {
	if len(servers) == 1 {
		return servers[0]
	}
	s.randMu.Lock()
	idx := s.rnd.Intn(len(servers))
	s.randMu.Unlock()
	return servers[idx]
}

func cloneLinkServers(src []g79client.G79LinkServerEntry) []g79client.G79LinkServerEntry {
	if len(src) == 0 {
		return nil
	}
	dst := make([]g79client.G79LinkServerEntry, len(src))
	copy(dst, src)
	return dst
}
