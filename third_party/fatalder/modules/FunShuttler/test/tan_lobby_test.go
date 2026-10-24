package test_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service"
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft"
)

func TestTanLobby(t *testing.T) {
	client, err := auth.CreateClient(&auth.ClientOptions{
		AuthServer: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	accessWrapper := auth.NewAccessWrapper(
		client,
		"949926",
		"",
		`{"sauth_json":"{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"pc\",\"sdkuid\":\"aibgraaeqoilupln\",\"sessionid\":\"1-eyJzIjogImh1cGgwd3RqbnZkMDhkZm80aGppeW1xbXExejd2Y2RkIiwgIm9kaSI6ICJhbWF3cmFhYWF3cjV0MmNnLWQiLCAic2kiOiAiYTA3YjRjZGQ5YzU2OWJhNDhkNTBhMzYyMGQyMWQzOTcxMmZmMTBhYiIsICJ1IjogImFpYmdyYWFlcW9pbHVwbG4iLCAidCI6IDIsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCJ9\",\"sdk_version\":\"3.9.0\",\"udid\":\"bdtt1w4wof8hh07l7awbsahq7ow1pfuu\",\"deviceid\":\"amawraaaawr5t2cg-d\",\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"client_login_sn\":\"B5204F04CE1E12B968881273A8859983\",\"gas_token\":\"\",\"source_platform\":\"pc\",\"ip\":\"127.0.0.1\"}"}`,
		"",
		"",
	)
	netConn, tanLobbyLoginResp, err := service.Dial(
		accessWrapper.GetRoomID(),
		accessWrapper.GetRoomPasscode(),
		accessWrapper.TanLobbyAuthenticator(),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn, err := minecraft.TanLobbyDialContext(context.Background(), netConn, tanLobbyLoginResp)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	for {
		pk, err := conn.ReadPacket()
		if err != nil {
			break
		}
		fmt.Println(pk)
	}
}
