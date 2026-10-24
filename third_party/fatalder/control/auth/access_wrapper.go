package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	tanauth "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	bunkerauth "github.com/Yeah114/FunShuttler/core/bunker/auth"
)

// AccessWrapper adapts control-layer configuration to the FunShuttler client expectations.
type AccessWrapper struct {
	ServerCode     string
	ServerPassword string
	Token          string
	Username       string
	Password       string
	Client         *Client
}

// NewAccessWrapper constructs an access wrapper bound to the provided auth client.
func NewAccessWrapper(client *Client, serverCode, serverPassword, token, username, password string) *AccessWrapper {
	return &AccessWrapper{
		Client:         client,
		ServerCode:     serverCode,
		ServerPassword: serverPassword,
		Token:          token,
		Username:       username,
		Password:       password,
	}
}

// GetAccess performs Phoenix authentication and persists FB tokens when provided.
func (aw *AccessWrapper) GetAccess(ctx context.Context, publicKey []byte) (bunkerauth.AuthResponse, error) {
	pubKey := base64.StdEncoding.EncodeToString(publicKey)
	resp, err := aw.Client.Auth(ctx, aw.ServerCode, aw.ServerPassword, pubKey, aw.Token, aw.Username, aw.Password)
	if err != nil {
		return bunkerauth.AuthResponse{}, err
	}
	if resp.FBToken != "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "."
		}
		configDir := filepath.Join(homeDir, ".config", "fastbuilder")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return bunkerauth.AuthResponse{}, err
		}
		tokenPath := filepath.Join(configDir, "fbtoken")
		f, err := os.OpenFile(tokenPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return bunkerauth.AuthResponse{}, err
		}
		if _, err := f.WriteString(resp.FBToken); err != nil {
			f.Close()
			return bunkerauth.AuthResponse{}, err
		}
		f.Close()
	}
	return resp, nil
}

// TanLobbyAuthenticator exposes the Tan Lobby authenticator implementation.
func (aw *AccessWrapper) TanLobbyAuthenticator() tanauth.Authenticator {
	return &tanLobbyAuthenticator{aw: aw}
}

// RentalServerCode returns the configured rental server code.
func (aw *AccessWrapper) RentalServerCode() string {
	return aw.ServerCode
}

// RentalServerPasscode returns the configured rental server passcode.
func (aw *AccessWrapper) RentalServerPasscode() string {
	return aw.ServerPassword
}

// AuthClient exposes the underlying auth client to the FunShuttler client.
func (aw *AccessWrapper) AuthClient() bunkerauth.ClientAPI {
	return aw.Client
}

// tanLobbyAuthenticator bridges Tan Lobby authentication requests to the client.
type tanLobbyAuthenticator struct {
	aw *AccessWrapper
}

func (t *tanLobbyAuthenticator) GetAccess(roomID string) (tanauth.TanLobbyLoginResponse, error) {
	resp, err := t.aw.Client.TanLobbyAuth(roomID, t.aw.Token)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, fmt.Errorf("TanLobbyGetAccess: %v", err)
	}
	return resp, nil
}

func (t *tanLobbyAuthenticator) GetCreate() (tanauth.TanLobbyCreateResponse, error) {
	resp, err := t.aw.Client.TanLobbyCreate(t.aw.Token)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, fmt.Errorf("TanLobbyGetCreate: %v", err)
	}
	return resp, nil
}

func (t *tanLobbyAuthenticator) GetDebug(loginResponse string, raknetRand []byte) (tanauth.TanLobbyDebugResponse, error) {
	resp, err := t.aw.Client.TanLobbyDebug(loginResponse, raknetRand, t.aw.Token)
	if err != nil {
		return tanauth.TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyGetDebug: %v", err)
	}
	return resp, nil
}
