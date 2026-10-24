package uqholder

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/uqholder/defines"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/vmihailenco/msgpack/v5"
)

func init() {
	if false {
		func(defines.ExtendInfo) {}(&ExtendInfoHolder{})
	}
}

// 包含窗口ID与请求变更的新物品信息
type ItemStackRequestDetails struct {
	WindowID             uint32
	SlotWithItemInstance map[uint8]protocol.ItemInstance
}

type ExtendInfoHolder struct {
	uq *MicroUQHolder
	CompressThreshold            uint16
	KnownCompressThreshold       bool
	LastSyncRatioStaticStartTime time.Time
	LastSyncRatioStaticStartTick int64
	SyncRatio                    float32
	CurrentTick                  int64
	KnownCurrentTick             bool
	WorldGameMode              int32
	KnownWorldGameMode      bool
	GameMode                  int32
	KnownGameMode           bool
	WorldDifficulty              uint32
	KnownWorldDifficulty         bool
	CurrentContainerOpened       bool
	CurrentOpenedContainer       *packet.ContainerOpen
	Time                         int32
	KnownTime                    bool
	DayTime                      int32
	KnownDayTime                 bool
	DayTimePercent               float32
	KnownDayTimePercent          bool
	GameRules                    map[string]*defines.GameRule
	KnownGameRules               bool
	Dimension                    int32
	KnownDimension               bool
	ClientDimension              int32
	ClientHoldingItem             protocol.ItemInstance
	ClientHotBarSlot               byte
	BotRuntimeIDDup              uint64
	PositionUpdateTick           int64
	Position                     mgl32.Vec3
}

func NewExtendInfoHolder(conn *minecraft.Conn) *ExtendInfoHolder {
	gameData := conn.GameData()
	return &ExtendInfoHolder{
		GameRules:          make(map[string]*defines.GameRule),
		BotRuntimeIDDup:    gameData.EntityRuntimeID,
		Position:           gameData.PlayerPosition,
		Dimension:          gameData.Dimension,
		KnownDimension:   true,
		PositionUpdateTick: gameData.Time,
		CurrentTick:        gameData.Time,
		WorldGameMode: gameData.WorldGameMode,
		KnownWorldGameMode: true,
		GameMode:       gameData.PlayerGameMode,
		KnownGameMode: true,
	}
}

// func (e *ExtendInfoHolder) GetWorldName() (worldName string, found bool) {
// 	return e.WorldName, e.knownWorldName
// }

// func (e *ExtendInfoHolder) setWorldName(worldName string) {
// 	e.WorldName = worldName
// 	e.knownWorldName = true
// }

func (e *ExtendInfoHolder) setUQ(uq *MicroUQHolder) {
	e.uq = uq
}

func (e *ExtendInfoHolder) GetCompressThreshold() (compressThreshold uint16, found bool) {
	return e.CompressThreshold, e.KnownCompressThreshold
}

func (e *ExtendInfoHolder) setCompressThreshold(compressThreshold uint16) {
	e.CompressThreshold = compressThreshold
	e.KnownCompressThreshold = true
}

func (e *ExtendInfoHolder) GetCurrentTick() (currentTick int64, found bool) {
	return e.CurrentTick, e.KnownCurrentTick
}

func (e *ExtendInfoHolder) setCurrentTick(currentTick int64) {
	e.CurrentTick = currentTick
	e.KnownCurrentTick = true
}

func (e *ExtendInfoHolder) GetWorldGameMode() (worldGameMode int32, found bool) {
	return e.WorldGameMode, e.KnownWorldGameMode
}

func (e *ExtendInfoHolder) setWorldGameMode(worldGameMode int32) {
	e.WorldGameMode = worldGameMode
	e.KnownWorldGameMode = true
}

func (e *ExtendInfoHolder) GetGameMode() (gameMode int32, found bool) {
	return e.GameMode, e.KnownGameMode
}

func (e *ExtendInfoHolder) setGameMode(gameMode int32) {
	e.GameMode = gameMode
	e.KnownGameMode = true
}

func (e *ExtendInfoHolder) GetWorldDifficulty() (worldDifficulty uint32, found bool) {
	return e.WorldDifficulty, e.KnownWorldDifficulty
}

func (e *ExtendInfoHolder) setWorldDifficulty(worldDifficulty uint32) {
	e.WorldDifficulty = worldDifficulty
	e.KnownWorldDifficulty = true
}

// func (e *ExtendInfoHolder) GetInventorySlotCount() (inventorySlotCount uint32, found bool) {
// 	return e.InventorySlotCount, e.knownInventorySlotCount
// }

// func (e *ExtendInfoHolder) setInventorySlotCount(inventorySlotCount uint32) {
// 	e.InventorySlotCount = inventorySlotCount
// 	e.knownInventorySlotCount = true
// }

func (e *ExtendInfoHolder) GetTime() (time int32, found bool) {
	return e.Time, e.KnownTime
}

func (e *ExtendInfoHolder) setTime(time int32) {
	e.Time = time
	e.KnownTime = true
}

func (e *ExtendInfoHolder) GetDayTime() (dayTime int32, found bool) {
	return e.DayTime, e.KnownDayTime
}

func (e *ExtendInfoHolder) setDayTime(dayTime int32) {
	e.DayTime = dayTime
	e.KnownDayTime = true
}

func (e *ExtendInfoHolder) GetDayTimePercent() (dayTimePercent float32, found bool) {
	return e.DayTimePercent, e.KnownDayTimePercent
}

func (e *ExtendInfoHolder) setDayTimePercent(dayTimePercent float32) {
	e.DayTimePercent = dayTimePercent
	e.KnownDayTimePercent = true
}

func (e *ExtendInfoHolder) GetGameRules() (gameRules map[string]*defines.GameRule, found bool) {
	return e.GameRules, e.KnownGameRules
}

func (e *ExtendInfoHolder) GetSyncRatio() (ratio float32, known bool) {
	return e.SyncRatio, e.SyncRatio == 0
}

func (e *ExtendInfoHolder) setGameRules(gameRuleName string, rule *defines.GameRule) {
	e.GameRules[gameRuleName] = rule
	e.KnownGameRules = true
}

func (e *ExtendInfoHolder) GetCurrentOpenedContainer() (container *packet.ContainerOpen, open bool) {
	return e.CurrentOpenedContainer, e.CurrentContainerOpened
}

func (e *ExtendInfoHolder) GetBotDimension() (dimension int32, found bool) {
	if e.KnownDimension {
		return e.Dimension, true
	} else {
		return 0, false
	}
}

func (e *ExtendInfoHolder) SetClientDimension(dimension int32) {
	e.ClientDimension = dimension
}

func (e *ExtendInfoHolder) GetClientDimension() (dimension int32) {
	return e.ClientDimension
}

func (e *ExtendInfoHolder) setClientHoldingItem(item protocol.ItemInstance) {
	e.ClientHoldingItem = item
}

func (e *ExtendInfoHolder) GetClientHoldingItem() (item protocol.ItemInstance) {
	return e.ClientHoldingItem
}

func (e *ExtendInfoHolder) setClientHotBarSlot(slot byte) {
	e.ClientHotBarSlot = slot
}

func (e *ExtendInfoHolder) GetClientHotBarSlot() (slot byte) {
	return e.ClientHotBarSlot
}

func (e *ExtendInfoHolder) GetBotPosition() (pos mgl32.Vec3, outOfSyncTick int64) {
	// though currently position is always known,
	// in future we may use it (found) to represent "out of sync" status
	// fmt.Printf("e.CurrentTick %v e.PositionUpdateTick %v\n", e.CurrentTick, e.PositionUpdateTick)
	outOfSyncTick = e.CurrentTick - e.PositionUpdateTick
	if outOfSyncTick < 0 {
		outOfSyncTick = 0
	}
	return e.Position, outOfSyncTick
}

func (uq *ExtendInfoHolder) UpdateFromPacket(pk packet.Packet) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Println("UQHolder Update Error: ", r)
			debug.PrintStack()
		}
	}()
	switch p := pk.(type) {
	case *packet.MobEquipment:
		uq.setClientHoldingItem(p.NewItem)
		uq.setClientHotBarSlot(p.HotBarSlot)
	case *packet.NetworkSettings:
		uq.setCompressThreshold(p.CompressionThreshold)
	case *packet.SetTime:
		uq.setTime(p.Time)
		uq.setDayTime(p.Time % 24000)
		uq.setDayTimePercent(float32(uq.DayTime) / 24000)
	case *packet.GameRulesChanged:
		for _, r := range p.GameRules {
			uq.setGameRules(r.Name, &defines.GameRule{
				CanBeModifiedByPlayer: r.CanBeModifiedByPlayer,
				Value:                 fmt.Sprintf("%v", r.Value),
			})
		}
	case *packet.SetDefaultGameType:
		uq.setWorldGameMode(p.GameType)
	case *packet.SetPlayerGameType:
		uq.setGameMode(p.GameType)
	case *packet.UpdatePlayerGameType:
		if uq.uq != nil && p.PlayerUniqueID == uq.uq.GetBotBasicInfo().GetBotUniqueID() {
			uq.setGameMode(p.GameType)
		}
	case *packet.SetDifficulty:
		uq.setWorldDifficulty(p.Difficulty)
	case *packet.TickSync:
		nowTime := time.Now()
		if p.ClientRequestTimestamp == 0 {
			uq.setCurrentTick(p.ServerReceptionTimestamp)
			// fmt.Println("tick sync", p)
		} else {
			deltaTime := p.ServerReceptionTimestamp - p.ClientRequestTimestamp
			if deltaTime < 0 {
				deltaTime = 0
			}
			uq.setCurrentTick(p.ServerReceptionTimestamp + deltaTime)
			if uq.LastSyncRatioStaticStartTick != 0 {
				ticksShouldGo := nowTime.Sub(uq.LastSyncRatioStaticStartTime).Milliseconds() / 50
				ticksActualGo := p.ServerReceptionTimestamp - uq.LastSyncRatioStaticStartTick
				syncRatio := float32(ticksActualGo) / float32(ticksShouldGo)
				if syncRatio > 1 {
					uq.SyncRatio = 1
				} else {
					uq.SyncRatio = syncRatio
				}
			}
			uq.LastSyncRatioStaticStartTick = p.ServerReceptionTimestamp
			uq.LastSyncRatioStaticStartTime = time.Now()
		}
	case *packet.ChangeDimension:
		uq.Dimension = p.Dimension
		uq.KnownDimension = true
		uq.Position = p.Position
		uq.PositionUpdateTick = uq.CurrentTick
	case *packet.PlayerAuthInput:
		uq.Position = p.Position
		uq.CurrentTick = int64(p.Tick) + 1
		uq.PositionUpdateTick = uq.CurrentTick
	case *packet.MovePlayer:
		// fmt.Println(p)
		if p.EntityRuntimeID == uq.BotRuntimeIDDup {
			// fmt.Println(p)
			uq.Position = p.Position
			// uq.CurrentTick = int64(p.Tick) + 1 p.Tick is 0
			uq.PositionUpdateTick = uq.CurrentTick
			// EntityRuntimeID:          1,
			// Position:                 p.Position,
			// Pitch:                    p.Pitch,
			// Yaw:                      p.Yaw,
			// HeadYaw:                  p.HeadYaw,
			// Mode:                     p.Mode,
			// OnGround:                 p.OnGround,
			// RiddenEntityRuntimeID:    p.RiddenEntityRuntimeID,
			// TeleportCause:            p.TeleportCause,
			// TeleportSourceEntityType: p.TeleportSourceEntityType,
			// Tick:                     o.Tick + 1,
		}

	case *packet.Respawn:
		if p.EntityRuntimeID == uq.BotRuntimeIDDup {
			uq.Position = p.Position
			uq.PositionUpdateTick = uq.CurrentTick
		}
	case *packet.CorrectPlayerMovePrediction:
		uq.Position = p.Position
		uq.CurrentTick = int64(p.Tick) + 1
		uq.PositionUpdateTick = uq.CurrentTick
	case *packet.ContainerOpen:
		uq.CurrentOpenedContainer = p
		uq.CurrentContainerOpened = true
	case *packet.ContainerClose:
		uq.CurrentOpenedContainer = nil
		uq.CurrentContainerOpened = false
	}

	botReader, found := uq.uq.PlayersInfoHolder.GetPlayerByName(uq.uq.GetBotName())
	if !found {
		return
	}
	bot := botReader.(*Player)
	position, tick := uq.GetBotPosition()
	bot.setPosition(position)
	bot.setTick(uint64(tick))
}

func (e *ExtendInfoHolder) Marshal() ([]byte, error) {
	return msgpack.Marshal(e)
}

func (e *ExtendInfoHolder) Unmarshal(data []byte) error {
	return msgpack.Unmarshal(data, e)
}
