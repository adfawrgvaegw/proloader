package uqholder

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/uqholder/defines"
	"github.com/Yeah114/FunShuttler/utils/sync_wrapper"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

func init() {
	if false {
		func(defines.PlayersInfoHolder) {}(&Players{})
	}
}

type Players struct {
	PlayersByUUID                *sync_wrapper.SyncKVMap[uuid.UUID, *Player]
	CachePlayersByEntityUniqueID *sync_wrapper.SyncKVMap[int64, *Player]
	CachePlayerByUsername        *sync_wrapper.SyncKVMap[string, *Player]
	PendingAddPlayerPacket       map[int64]*packet.AddPlayer
	PendingAbilityData           map[int64]protocol.AbilityData
	PendingMu                    sync.Mutex
}

func NewPlayers() *Players {
	return &Players{
		PlayersByUUID:                sync_wrapper.NewSyncKVMap[uuid.UUID, *Player](),
		CachePlayersByEntityUniqueID: sync_wrapper.NewSyncKVMap[int64, *Player](),
		CachePlayerByUsername:        sync_wrapper.NewSyncKVMap[string, *Player](),
		PendingAddPlayerPacket:       make(map[int64]*packet.AddPlayer),
		PendingAbilityData:           make(map[int64]protocol.AbilityData),
		PendingMu:                    sync.Mutex{},
	}
}

func (p *Players) GetPlayerByUUID(uuid uuid.UUID) (player defines.PlayerUQReader, found bool) {
	return p.PlayersByUUID.Get(uuid)
}

func (p *Players) GetPlayerByUUIDString(uuidStr string) (player defines.PlayerUQReader, found bool) {
	uuid := uuid.UUID{}
	err := uuid.UnmarshalText([]byte(uuidStr))
	if err != nil {
		return nil, false
	}
	return p.GetPlayerByUUID(uuid)
}

func (p *Players) GetPlayerByUniqueID(uniqueID int64) (player defines.PlayerUQReader, found bool) {
	player, found = p.CachePlayersByEntityUniqueID.Get(uniqueID)
	if found {
		uid, hasUID := player.GetUUID()
		euid, hasEuid := player.GetEntityUniqueID()
		if hasEuid && hasUID {
			if player, found := p.GetPlayerByUUID(uid); found && euid == uniqueID {
				return player, found
			}
		}
		p.CachePlayersByEntityUniqueID.Delete(uniqueID)
	}
	p.PlayersByUUID.Iter(func(_uid uuid.UUID, _player *Player) bool {
		if _player.EntityUniqueID == uniqueID {
			p.CachePlayersByEntityUniqueID.Set(uniqueID, _player)
			player = _player
			found = true
			return false
		}
		return true
	})
	return
}

func (p *Players) GetPlayerByName(username string) (player defines.PlayerUQReader, found bool) {
	player, found = p.CachePlayerByUsername.Get(username)
	if found {
		uid, hasUID := player.GetUUID()
		uname, hasUname := player.GetUsername()
		if hasUname && hasUID {
			if player, found := p.GetPlayerByUUID(uid); found && uname == username {
				return player, found
			}
		}
		p.CachePlayerByUsername.Delete(username)
	}
	p.PlayersByUUID.Iter(func(_uid uuid.UUID, _player *Player) bool {
		if _player.Username == username {
			p.CachePlayerByUsername.Set(username, _player)
			player = _player
			found = true
			return false
		}
		return true
	})
	return
}

func (p *Players) GetPlayerByEntityRuntimeID(entityRuntimeID uint64) (player defines.PlayerUQReader, found bool) {
	p.PlayersByUUID.Iter(func(_uuid uuid.UUID, _player *Player) bool {
		if _player.EntityRuntimeID == entityRuntimeID {
			player = _player
			found = true
			return false
		}
		return true
	})
	return
}

func (players *Players) Marshal() ([]byte, error) {
	playersByUUID := make(map[uuid.UUID]*Player)
	players.PlayersByUUID.Iter(func(_uuid uuid.UUID, _player *Player) bool {
		playersByUUID[_uuid] = _player
		return true
	})
	return msgpack.Marshal(playersByUUID)
}

func (players *Players) Unmarshal(data []byte) error {
	return msgpack.Unmarshal(data, &players)
}

func GetStringContents(s string) []string {
	_s := strings.Split(s, " ")
	for i, c := range _s {
		_s[i] = strings.TrimSpace(c)
	}
	ss := make([]string, 0, len(_s))
	for _, c := range _s {
		if c != "" {
			ss = append(ss, c)
		}
	}
	return ss
}

const (
	automataStateBeginningOfWord = 0
	automataStateTakeRune        = 1
)

func ToPlainName(name string) string {
	if !strings.ContainsAny(name, "<>§") {
		return name
	}
	flip := false
	cleanedNameRunes := []rune{}
	for _, r := range name {
		if flip {
			flip = false
			continue
		} else if r == '§' {
			flip = true
			continue
		}
		cleanedNameRunes = append(cleanedNameRunes, r)
	}
	cleanedName := string(cleanedNameRunes)
	if !strings.ContainsAny(cleanedName, "<>") {
		return cleanedName
	}

	automataState := automataStateBeginningOfWord
	lastWord := []rune{}
	for _, r := range cleanedName {
		leftBracket := r == '<'
		rightBracket := r == '>'
		switch automataState {
		case automataStateBeginningOfWord:
			if leftBracket || rightBracket {
				continue
			} else {
				lastWord = []rune{r}
				automataState = automataStateTakeRune
			}
		case automataStateTakeRune:
			if leftBracket || rightBracket {
				automataState = automataStateBeginningOfWord
			} else {
				lastWord = append(lastWord, r)
			}
		}
	}
	return string(lastWord)
}

func (p *Players) AddPlayer(e protocol.PlayerListEntry) *Player {
	player := NewPlayerUQHolder()
	player.setUUID(e.UUID)
	player.setEntityUniqueID(e.EntityUniqueID)
	player.setUsername(ToPlainName(e.Username))
	player.setXUID(e.XUID)
	player.setPlatformChatID(e.PlatformChatID)
	player.setBuildPlatform(e.BuildPlatform)
	player.setSkinID(e.Skin.SkinID)
	player.setLoginTime(time.Now())

	p.PlayersByUUID.Set(e.UUID, player)
	p.CachePlayersByEntityUniqueID.Set(e.EntityUniqueID, player)
	p.CachePlayerByUsername.Set(ToPlainName(e.Username), player)
	return player
}

func (p *Players) RemovePlayer(e protocol.PlayerListEntry) {
	if player, ok := p.PlayersByUUID.Get(e.UUID); ok {
		player.Online = false
		p.CachePlayersByEntityUniqueID.Delete(player.EntityUniqueID)
		p.CachePlayerByUsername.Delete(player.Username)
		p.PlayersByUUID.Delete(e.UUID)
	} else {
		println("player not found: ", e.UUID.String())
	}
}

func (uq *Players) updateAbilityData(ability protocol.AbilityData) {
	uniqueID := ability.EntityUniqueID
	playerReader, found := uq.GetPlayerByUniqueID(uniqueID)
	if !found {
		uq.PendingMu.Lock()
		uq.PendingAbilityData[uniqueID] = ability
		uq.PendingMu.Unlock()
		return
	}
	player := playerReader.(*Player)
	player.UpdateAbility(ability)
	/*
	for _, layer := range ability.Layers {
		if layer.Type == protocol.AbilityLayerTypeBase {
			Values := layer.Values
			player.KnowAbilitiesAndStatus = true
			// 下面三行同步改为大写+Field后缀
			player.StatusInvulnerableField = (Values & protocol.AbilityInvulnerable) != 0
			player.StatusFlyingField = (Values & protocol.AbilityFlying) != 0
			player.StatusMayFlyField = (Values & protocol.AbilityMayFly) != 0
		}
	}
	*/
}

func (uq *Players) UpdateFromPacket(pk packet.Packet) {
	// if pk.ID() == packet.IDAdventureSettings || pk.ID() == packet.IDPlayerList {
	// 	bs, _ := json.Marshal(pk)
	// 	fmt.Println(string(bs))
	// }

	defer func() {
		r := recover()
		if r != nil {
			println("UQHolder Update Error: ", r)
			// debug.PrintStack()
		}
	}()
	switch p := pk.(type) {
	case *packet.PlayerList:
		if p.ActionType == packet.PlayerListActionAdd {
			for _, e := range p.Entries {
				player := uq.AddPlayer(e)
				uq.PendingMu.Lock()
				if pk, found := uq.PendingAbilityData[e.EntityUniqueID]; found {
					uq.updateAbilityData(pk)
					delete(uq.PendingAbilityData, e.EntityUniqueID)
				}
				if pk, found := uq.PendingAddPlayerPacket[e.EntityUniqueID]; found {
					player.setDeviceID(pk.DeviceID)
					player.setEntityRuntimeID(pk.EntityRuntimeID)
					player.setEntityMetadata(pk.EntityMetadata)
					player.setPosition(pk.Position)
					player.setPitch(pk.Pitch)
					player.setYaw(pk.Yaw)
					player.setHeadYaw(pk.HeadYaw)
					delete(uq.PendingAddPlayerPacket, e.EntityUniqueID)
				}
				uq.PendingMu.Unlock()
			}
		} else {
			for _, e := range p.Entries {
				uq.RemovePlayer(e)
			}
		}
	// case *packet.UpdateAdventureSettings:
	case *packet.UpdateAbilities:
		uq.updateAbilityData(p.AbilityData)
	// case *packet.AdventureSettings:
	// 	playerReader, found := uq.GetPlayerByUniqueID(p.PlayerUniqueID)
	// 	if !found {
	// 		uq.PendingMu.Lock()
	// 		uq.PendingAdventureSettingsPacket[p.PlayerUniqueID] = p
	// 		uq.PendingMu.Unlock()
	// 		return
	// 	}
	// 	player := playerReader.(*Player)
	// 	player.setPropertiesFlag(p.Flags)
	// 	player.setCommandPermissionLevel(p.CommandPermissionLevel)
	// 	player.setActionPermissions(p.ActionPermissions)
	// 	player.setOPPermissionLevel(p.PermissionLevel)
	// 	player.setCustomStoredPermissions(p.CustomStoredPermissions)
	case *packet.AddPlayer:
		uq.updateAbilityData(p.AbilityData)
		playerReader, found := uq.GetPlayerByUniqueID(p.AbilityData.EntityUniqueID)
		if !found {
			uq.PendingMu.Lock()
			uq.PendingAddPlayerPacket[p.AbilityData.EntityUniqueID] = p
			uq.PendingMu.Unlock()
			return
		}
		player := playerReader.(*Player)
		player.setDeviceID(p.DeviceID)
		player.setEntityRuntimeID(p.EntityRuntimeID)
		player.setEntityMetadata(p.EntityMetadata)
		player.setPosition(p.Position)
		player.setPitch(p.Pitch)
		player.setYaw(p.Yaw)
		player.setHeadYaw(p.HeadYaw)
	case *packet.MovePlayer:
		playerReader, found := uq.GetPlayerByEntityRuntimeID(p.EntityRuntimeID)
		if !found {
			return
		}
		player := playerReader.(*Player)
		position := p.Position
		position[0] -= 1
		position[1] -= 2
		player.setPosition(position)
		player.setPitch(p.Pitch)
		player.setYaw(p.Yaw)
		player.setHeadYaw(p.HeadYaw)
		player.setOnGround(p.OnGround)
		player.setMode(p.Mode)
		player.setRiddenEntityRuntimeID(p.RiddenEntityRuntimeID)
	case *packet.PyRpc:
		// [ModEventS2C [Minecraft chatExtension PlayerAddRoom map[id2DimId:map[-25769000000:0] id2Uid:map[-25769000000:2149000000] prefixInfo:map[-25769000000:map[]] uids:[2149000000]]] <nil>]
		valueList, ok := p.Value.([]any)
		if !ok || len(valueList) != 3 {
			return
		}
		// Convert first item to string and check event type
		eventType, ok := valueList[0].(string)
		if !ok || eventType != "ModEventS2C" {
			return
		}
		// Convert second item to []any and check length
		contentList, ok := valueList[1].([]any)
		if !ok || len(contentList) != 4 {
			return
		}
		// Check event name
		eventName, ok := contentList[2].(string)
		if !ok || eventName != "PlayerAddRoom" {
			return
		}
		// Convert event data to map[string]any and get details
		eventData, ok := contentList[3].(map[any]any)
		if !ok {
			return
		}
		// Convert id2Uid to map[string]any
		id2Uid, ok := eventData["id2Uid"].(map[any]any)
		if !ok {
			return
		}
		for sUid, aNeteaseUid := range id2Uid {
			sNeteaseUid, ok := aNeteaseUid.(string)
			if !ok {
				continue
			}
			uid, err := strconv.ParseInt(sUid.(string), 10, 64)
			if err != nil {
				continue
			}
			neteaseUid, err := strconv.ParseInt(sNeteaseUid, 10, 64)
			if err != nil {
				continue
			}
			playerReader, found := uq.GetPlayerByUniqueID(uid)
			if !found {
				continue
			}
			player := playerReader.(*Player)
			player.setNeteaseUID(neteaseUid)
		}
	}
}

func (players *Players) GetAllOnlinePlayers() []defines.PlayerUQReader {
	playersOut := make([]defines.PlayerUQReader, 0)
	players.PlayersByUUID.Iter(func(_uuid uuid.UUID, _player *Player) bool {
		playersOut = append(playersOut, _player)
		return true
	})
	return playersOut
}
