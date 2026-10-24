package minecraft

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"log"
	"net"
	"os"
	"time"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
)

func ToConn(netConn net.Conn) (*Conn, *packet.Decoder, func([]byte) error) {
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	conn := newConn(
		netConn,
		false,
		key,
		log.New(os.Stderr, "", log.LstdFlags),
		DefaultProtocol,
		time.Second/20,
		false,
	)
	conn.TanLobbyListener = true
	conn.acceptedProto = append(conn.acceptedProto, DefaultProtocol)
	conn.pool = DefaultProtocol.Packets(true)
	conn.compression = packet.NeteaseCompression()
	return conn, conn.dec, conn.receive
}

func (c *Conn) LoggedIn() bool {
	return c.loggedIn
}