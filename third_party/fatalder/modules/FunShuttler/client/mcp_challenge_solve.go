package client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/core/py_rpc"
	"github.com/google/uuid"
)

// CopeChallenge ..
func (m *MCPCheckChallengesSolver) CopeChallenge() error {
	err := m.solveMCPCheckChallenges()
	if err != nil {
		return fmt.Errorf("CopeChallenge: %v", err)
	}
	return nil
}

// solveMCPCheckChallenges ..
func (m *MCPCheckChallengesSolver) solveMCPCheckChallenges() error {
	// prepare
	var (
		pk               packet.Packet             = nil
		err              error                     = nil
		challengeTimeout bool                      = false
		challengeError   chan struct{}             = make(chan struct{})
		challengeSolved  chan struct{}             = make(chan struct{})
		cachedPkt        chan packet.Packet        = make(chan packet.Packet, 32767)
		commandOutput    chan packet.CommandOutput = make(chan packet.CommandOutput, 1)
		timer            *time.Timer               = time.NewTimer(time.Second * 30)
	)

	// read packet and process
	go func() {
		for {
			// challenge timeout
			if challengeTimeout {
				return
			}

			// read packet
			pk, err = m.client.connection.ReadPacket()
			if !challengeTimeout && err != nil {
				close(challengeError)
				return
			}

			cachedPkt <- pk
			// for each incoming packet
			switch p := pk.(type) {
			case *packet.PyRpc:
				olderStates := m.client.getCheckNumEverPassed
				if err = m.onPyRpc(p); err != nil {
					close(challengeError)
					return
				}
				if !olderStates && m.client.getCheckNumEverPassed {
					close(challengeSolved)
				}
			case *packet.CommandOutput:
				commandOutput <- *p
				return
			}
		}
	}()

	// wait for the challenge to end
	select {
	case <-challengeSolved:
		err = m.waitMCPCheckChallengesDown(commandOutput)
		close(cachedPkt)
		if err != nil {
			return fmt.Errorf("solveMCPCheckChallenges: %v", err)
		}
		m.client.cachedPacket = cachedPkt
		return nil
	case <-challengeError:
		close(challengeSolved)
		close(cachedPkt)
		return fmt.Errorf("solveMCPCheckChallenges: %v", err)
	case <-timer.C:
		challengeTimeout = true
		return fmt.Errorf("solveMCPCheckChallenges: Failed to pass the MCPC check challenges, please try again later")
	}
}

// waitMCPCheckChallengesDown ..
func (m *MCPCheckChallengesSolver) waitMCPCheckChallengesDown(commandOutput chan packet.CommandOutput) error {
	ticker := time.NewTicker(time.Second / 1)
	defer ticker.Stop()

	for {
		err := m.client.connection.WritePacket(&packet.CommandRequest{
			CommandLine: "list",
			CommandOrigin: protocol.CommandOrigin{
				Origin:    protocol.CommandOriginAutomationPlayer,
				UUID:      uuid.New(),
				RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694",
			},
			Internal: false,
			// PhoenixBuilder specific changes.
			// Author: LNSSPsd
			UnLimited: false,
			// PhoenixBuilder specific changes.
			// Author: Liliya233
			Version: 0x24,
		})

		if err != nil {
			return fmt.Errorf("waitMCPCheckChallengesDown: %v", err)
		}

		select {
		case <-commandOutput:
			close(commandOutput)
			return nil
		case <-ticker.C:
		}
	}
}

// onPyRpc ..
func (m *MCPCheckChallengesSolver) onPyRpc(p *packet.PyRpc) error {
	// prepare
	conn := m.client.connection
	client := m.client.authClient
	if p.Value == nil {
		return nil
	}

	// unmarshal
	content, err := py_rpc.Unmarshal(p.Value)
	if err != nil {
		return fmt.Errorf("onPyRpc: %v", err)
		//return nil
	}

	// do some actions for some specific PyRpc packets
	switch c := content.(type) {
	case *py_rpc.StartType:
		// get data and send packet
		c.Content, err = client.TransferData(c.Content)
		if err != nil {
			return fmt.Errorf("onPyRpc: %v", err)
		}
		c.Type = py_rpc.StartTypeResponse
		conn.WritePacket(&packet.PyRpc{
			Value:         py_rpc.Marshal(c),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	case *py_rpc.GetMCPCheckNum:
		// if the challenges has been down,
		// then we do NOTHING
		if m.client.getCheckNumEverPassed {
			break
		}
		// create request to the auth server and get response
		arg, _ := json.Marshal([]any{
			c.FirstArg,
			c.SecondArg.Arg,
			conn.GameData().EntityUniqueID,
		})
		ret, err := client.TransferCheckNum(auth.FNumRequest{Data: string(arg)})
		if err != nil {
			return fmt.Errorf("onPyRpc: %v", err)
		}
		// unmarshal response and adjust the data included
		ret_p := []any{}
		json.Unmarshal([]byte(ret), &ret_p)
		if len(ret_p) > 7 {
			ret6, ok := ret_p[6].(float64)
			if ok {
				ret_p[6] = int64(ret6)
			}
		}
		// send packet and mark this challenges was finished
		conn.WritePacket(&packet.PyRpc{
			Value:         py_rpc.Marshal(&py_rpc.SetMCPCheckNum{ret_p}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
		m.client.getCheckNumEverPassed = true
	}

	// return
	return nil
}
