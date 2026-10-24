package client

import (
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
)

// ------------------------- Config -------------------------

// Config ..
type Config struct {
	CacheAllPacket       bool
	AuthServerAddress    string
	AuthServerToken      string
	RentalServerCode     string
	RentalServerPasscode string
	AccessWrapper        AccessWrapper
}

// ------------------------- Client -------------------------

// Client ..
type Client struct {
	connection            *minecraft.Conn
	authClient            auth.ClientAPI
	authenticator         AccessWrapper
	getCheckNumEverPassed bool
	cachedPacket          chan packet.Packet
	CacheAllPacket        bool
}

// Conn ..
func (c Client) Conn() *minecraft.Conn {
	return c.connection
}

// Authenticator ..
func (c Client) Authenticator() AccessWrapper {
	return c.authenticator
}

// Auth ..
func (c Client) Auth() auth.ClientAPI {
	return c.authClient
}

// CachedPacket ..
func (c Client) CachedPacket() chan packet.Packet {
	return c.cachedPacket
}

// SetConn ...
func (c Client) SetConn(conn *minecraft.Conn) {
	c.connection = conn
}

// ------------------------- MCPCheckChallengesSolver -------------------------

// MCPCheckChallengesSolver ..
type MCPCheckChallengesSolver struct {
	client *Client
}

// NewChallengeSolver ..
func NewChallengeSolver(client *Client) *MCPCheckChallengesSolver {
	return &MCPCheckChallengesSolver{client: client}
}
