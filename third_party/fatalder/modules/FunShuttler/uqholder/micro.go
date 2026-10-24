package uqholder

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/uqholder/defines"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	DEBUG = false
)

func init() {
	if false {
		func(holder defines.MicroUQHolder) {}(&MicroUQHolder{})
	}
}

type MicroUQHolder struct {
	defines.BotBasicInfoHolder
	defines.PlayersInfoHolder
	defines.ExtendInfo
	mu                 sync.Mutex
}

// GetAllOnlinePlayers implements defines.MicroUQHolder interface.
func (u *MicroUQHolder) GetAllOnlinePlayers() []defines.PlayerUQReader {
	if u.PlayersInfoHolder != nil {
		return u.PlayersInfoHolder.GetAllOnlinePlayers()
	}
	return nil
}

func NewMicroUQHolder(conn *minecraft.Conn) *MicroUQHolder {
	extend_info := NewExtendInfoHolder(conn)
	uq := &MicroUQHolder{
		NewBotInfoHolder(conn).(*BotBasicInfoHolder),
		NewPlayers(),
		extend_info,
		sync.Mutex{},
	}
	extend_info.setUQ(uq)
	return uq
}

func (u *MicroUQHolder) GetBotBasicInfo() defines.BotBasicInfoHolder {
	return u.BotBasicInfoHolder
}

func (u *MicroUQHolder) GetPlayersInfo() defines.PlayersInfoHolder {
	return u.PlayersInfoHolder
}

func (u *MicroUQHolder) GetExtendInfo() defines.ExtendInfo {
	return u.ExtendInfo
}

func (u *MicroUQHolder) Marshal() (data []byte, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	defer func() {
		if err != nil {
			fmt.Println(err)
		}
	}()
	botBasicInfoHolderBytes, err := u.BotBasicInfoHolder.Marshal()
	if err != nil {
		return nil, err
	}
	playersInfoHolderBytes, err := u.PlayersInfoHolder.Marshal()
	if err != nil {
		return nil, err
	}
	extendInfoBytes, err := u.ExtendInfo.Marshal()
	if err != nil {
		return nil, err
	}
	// 直接用msgpack序列化整个结构体
	uqholderData := map[string][]byte{
		"BotBasicInfoHolder": botBasicInfoHolderBytes,
		"PlayersInfoHolder":  playersInfoHolderBytes,
		"ExtendInfo":         extendInfoBytes,
	}
	return msgpack.Marshal(uqholderData)
}

var ErrInvalidUQHolderEntry = errors.New("invalid uqholder entry")

func (u *MicroUQHolder) Unmarshal(data []byte) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
	}()
	uqholderData := map[string][]byte{}
	if err := msgpack.Unmarshal(data, &uqholderData); err != nil {
		return err
	}
	if _, ok := uqholderData["BotBasicInfoHolder"]; !ok {
		return ErrInvalidUQHolderEntry
	}
	if _, ok := uqholderData["PlayersInfoHolder"]; !ok {
		return ErrInvalidUQHolderEntry
	}
	if _, ok := uqholderData["ExtendInfo"]; !ok {
		return ErrInvalidUQHolderEntry
	}
	if err := u.BotBasicInfoHolder.Unmarshal(uqholderData["BotBasicInfoHolder"]); err != nil {
		return err
	}
	if err := u.PlayersInfoHolder.Unmarshal(uqholderData["PlayersInfoHolder"]); err != nil {
		return err
	}
	if err := u.ExtendInfo.Unmarshal(uqholderData["ExtendInfo"]); err != nil {
		return err
	}
	return nil
}

func (u *MicroUQHolder) UpdateFromPacket(packet packet.Packet) {
	u.mu.Lock()
	defer u.mu.Unlock()
	// fmt.Println(packet)
	u.BotBasicInfoHolder.UpdateFromPacket(packet)
	u.PlayersInfoHolder.UpdateFromPacket(packet)
	u.ExtendInfo.UpdateFromPacket(packet)
}
