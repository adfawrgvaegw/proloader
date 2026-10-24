package client

import (
	"context"

	tanbunker "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	bunkerauth "github.com/Yeah114/FunShuttler/core/bunker/auth"
)

// AccessWrapper defines the behaviours required by the client package to
// interact with a rental server authenticator.
type AccessWrapper interface {
	GetAccess(ctx context.Context, publicKey []byte) (bunkerauth.AuthResponse, error)
	TanLobbyAuthenticator() tanbunker.Authenticator
	RentalServerCode() string
	RentalServerPasscode() string
	AuthClient() bunkerauth.ClientAPI
}
