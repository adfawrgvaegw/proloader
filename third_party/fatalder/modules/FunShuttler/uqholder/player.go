package uqholder

import (
	"time"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/uqholder/defines"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/go-gl/mathgl/mgl32"
)

func init() {
	if false {
		func(defines.PlayerUQReader) {}(&Player{})
	}
}

type Player struct {
	UUID                     uuid.UUID
	KnownUUID                bool
	EntityUniqueID           int64
	KnownEntityUniqueID      bool
	NeteaseUID               int64
	KnownNeteaseUID          bool
	LoginTime                time.Time
	KnownLoginTime           bool
	Username                 string
	KnownUsername            bool
	XUID                     string
	KnownXUID                bool
	PlatformChatID           string
	KnownPlatformChatID      bool
	BuildPlatform            int32
	KnownBuildPlatform       bool
	SkinID                   string
	KnownSkinID              bool
	/*
	KnowAbilitiesAndStatus   bool
	CanBuildField            bool
	CanMineField             bool
	CanDoorsAndSwitchesField bool
	CanOpenContainersField   bool
	CanAttackPlayersField    bool
	CanAttackMobsField       bool
	CanOperatorCommandsField bool
	CanTeleportField         bool
	StatusInvulnerableField  bool
	StatusFlyingField        bool
	StatusMayFlyField        bool
	*/
	Abilities                   uint32
	KnownAbilities            bool
	Values                    uint32
	KnownValues              bool
	FlySpeed                  float32
	KnownFlySpeed           bool
	WalkSpeed                float32
	KnownWalkSpeed         bool
	CommandPermissions     byte
	KnownCommandPermissions bool
	PlayerPermissions         byte
	KnownPlayerPermissions bool
	DeviceID                 string
	KnownDeviceID            bool
	EntityRuntimeID          uint64
	KnownEntityRuntimeID     bool
	EntityMetadata           map[uint32]any
	KnownEntityMetadata      bool
	Online                   bool
	Position				   mgl32.Vec3
	KnownPosition 			  bool
	Pitch   				   float32
	KnownPitch			   bool
	Yaw 					   float32
	KnownYaw				   bool
	HeadYaw                  float32
	KnownHeadYaw             bool
	Mode                     byte
	KnownMode			   bool
	OnGround 				   bool
	KnownOnGround		   bool
	RiddenEntityRuntimeID   uint64
	KnownRiddenEntityRuntimeID bool
	Tick 				   uint64
	KnownTick			   bool
}

func (p *Player) Marshal() ([]byte, error) {
	return msgpack.Marshal(p)
}

func (p *Player) Unmarshal(data []byte) error {
	return msgpack.Unmarshal(data, p)
}

func NewPlayerUQHolder() *Player {
	return &Player{
		Online: true,
	}
}

func (p *Player) StillOnline() bool {
	return p.Online
}

func (p *Player) GetUUID() (id uuid.UUID, found bool) {
	if p == nil {
		return
	}
	return p.UUID, p.KnownUUID
}

func (p *Player) GetUUIDString() (id string, found bool) {
	if p == nil {
		return
	}
	return p.UUID.String(), p.KnownUUID
}

func (p *Player) setUUID(id uuid.UUID) {
	p.UUID = id
	p.KnownUUID = true
}

func (p *Player) GetEntityUniqueID() (id int64, found bool) {
	if p == nil {
		return
	}
	return p.EntityUniqueID, p.KnownEntityUniqueID
}

func (p *Player) setEntityUniqueID(id int64) {
	p.EntityUniqueID = id
	p.KnownEntityUniqueID = true
}

func (p *Player) GetNeteaseUID() (id int64, found bool) {
	if p == nil {
		return
	}
	return p.NeteaseUID, p.KnownNeteaseUID
}

func (p *Player) setNeteaseUID(id int64) {
	p.NeteaseUID = id
	p.KnownNeteaseUID = true
}

func (p *Player) GetLoginTime() (t time.Time, found bool) {
	if p == nil {
		return
	}
	return p.LoginTime, p.KnownLoginTime
}

func (p *Player) setLoginTime(t time.Time) {
	p.LoginTime = t
	p.KnownLoginTime = true
}

func (p *Player) GetUsername() (name string, found bool) {
	if p == nil {
		return
	}
	return p.Username, p.KnownUsername
}

func (p *Player) setUsername(name string) {
	p.Username = name
	p.KnownUsername = true
}

func (p *Player) GetXUID() (xuid string, found bool) {
	if p == nil {
		return
	}
	return p.XUID, p.KnownXUID
}

func (p *Player) setXUID(xuid string) {
	p.XUID = xuid
	p.KnownXUID = true
}

func (p *Player) GetPlatformChatID() (id string, found bool) {
	if p == nil {
		return
	}
	return p.PlatformChatID, p.KnownPlatformChatID
}

func (p *Player) setPlatformChatID(id string) {
	p.PlatformChatID = id
	p.KnownPlatformChatID = true
}

func (p *Player) GetBuildPlatform() (platform int32, found bool) {
	if p == nil {
		return
	}
	return p.BuildPlatform, p.KnownBuildPlatform
}

func (p *Player) setBuildPlatform(platform int32) {
	p.BuildPlatform = platform
	p.KnownBuildPlatform = true
}

func (p *Player) GetSkinID() (id string, found bool) {
	if p == nil {
		return
	}
	return p.SkinID, p.KnownSkinID
}

func (p *Player) setSkinID(id string) {
	p.SkinID = id
	p.KnownSkinID = true
}

/*
func (player *Player) UpdateAbility(ability uint16) {
	// 赋值时也用 Field 字段
	player.CanBuildField = (ability & protocol.AbilityBuild) != 0
	player.CanMineField = (ability & protocol.AbilityMine) != 0
	player.CanDoorsAndSwitchesField = (ability & protocol.AbilityDoorsAndSwitches) != 0
	player.CanOpenContainersField = (ability & protocol.AbilityOpenContainers) != 0
	player.CanAttackPlayersField = (ability & protocol.AbilityAttackPlayers) != 0
	player.CanAttackMobsField = (ability & protocol.AbilityAttackMobs) != 0
	player.CanOperatorCommandsField = (ability & protocol.AbilityOperatorCommands) != 0
	player.CanTeleportField = (ability & protocol.AbilityTeleport) != 0
}
*/

func (player *Player) UpdateAbility(ability protocol.AbilityData) {
	player.setCommandPermissions(ability.CommandPermissions)
	player.setPlayerPermissions(ability.PlayerPermissions)
	for _, layer := range ability.Layers {
		player.setAbilities(layer.Abilities)
		player.setValues(layer.Values)
		player.setFlySpeed(layer.FlySpeed)
		player.setWalkSpeed(layer.WalkSpeed)
	}
}

// func (p *Player) GetPropertiesFlag() (flag uint32, found bool) {
// 	if p == nil {
// 		return
// 	}
// 	return p.PropertiesFlag, p.knownPropertiesFlag
// }

// func (p *Player) setPropertiesFlag(flag uint32) {
// 	p.PropertiesFlag = flag
// 	p.knownPropertiesFlag = true
// }

func (p *Player) GetCommandPermissions() (permissions byte, found bool) {
	if p == nil {
		return
	}
 	return p.CommandPermissions, p.KnownCommandPermissions
}

func (p *Player) setCommandPermissions(permissions byte) {
	p.CommandPermissions = permissions
	p.KnownCommandPermissions = true
}

func (p *Player) GetPlayerPermissions() (permissions byte, found bool) {
	if p == nil {
		return
	}
	return p.PlayerPermissions, p.KnownPlayerPermissions
}

func (p *Player) setPlayerPermissions(permissions byte) {
	p.PlayerPermissions = permissions
	p.KnownPlayerPermissions = true
}

func (p *Player) GetAbilities() (abilities uint32, found bool) {
	if p == nil {
		return
	}
	return p.Abilities, p.KnownAbilities
}

func (p *Player) setAbilities(abilities uint32) {
	p.Abilities = abilities
	p.KnownAbilities = true
}

func (p *Player) GetValues() (values uint32, found bool) {
	if p == nil {
		return
	}
	return p.Values, p.KnownValues
}

func (p *Player) setValues(values uint32) {
	p.Values = values
	p.KnownValues = true
}

func (p *Player) GetFlySpeed() (speed float32, found bool) {
	if p == nil {
		return
	}
	return p.FlySpeed, p.KnownFlySpeed
}

func (p *Player) setFlySpeed(speed float32) {
	p.FlySpeed = speed
	p.KnownFlySpeed = true
}

func (p *Player) GetWalkSpeed() (speed float32, found bool) {
	if p == nil {
		return
	}
	return p.WalkSpeed, p.KnownWalkSpeed
}

func (p *Player) setWalkSpeed(speed float32) {
	p.WalkSpeed = speed
	p.KnownWalkSpeed = true
}

// func (p *Player) GetOPPermissionLevel() (level uint32, found bool) {
// 	if p == nil {
// 		return
// 	}
// 	return p.OPPermissionLevel, p.knownOPPermissionLevel
// }

// func (p *Player) IsOP() (op bool, found bool) {
// 	if p == nil {
// 		return false, false
// 	}
// 	if p.knownOPPermissionLevel {
// 		if p.OPPermissionLevel == packet.PermissionLevelOperator {
// 			return true, true
// 		}
// 		if p.OPPermissionLevel < packet.PermissionLevelOperator {
// 			return false, true
// 		}
// 	}
// 	permission, found := p.GetCommandPermissionLevel()
// 	if found {
// 		if permission >= packet.CommandPermissionLevelHost {
// 			return true, true
// 		} else {
// 			return false, true
// 		}
// 	}

// 	permission, found = p.GetActionPermissions()
// 	isOP := (permission & packet.ActionPermissionOperator) != 0
// 	return isOP, found
// }

// func (p *Player) setOPPermissionLevel(level uint32) {
// 	p.OPPermissionLevel = level
// 	p.knownOPPermissionLevel = true
// }

// func (p *Player) GetCustomStoredPermissions() (permissions uint32, found bool) {
// 	if p == nil {
// 		return
// 	}
// 	return p.CustomStoredPermissions, p.knownCustomStoredPermissions
// }

// func (p *Player) setCustomStoredPermissions(permissions uint32) {
// 	p.CustomStoredPermissions = permissions
// 	p.knownCustomStoredPermissions = true
// }

/*
func (p *Player) IsOP() (op bool, found bool) {
	if p == nil {
		return false, false
	}
	if p.KnowAbilitiesAndStatus {
		return p.CanOperatorCommandsField, true

	}
	return false, false
}
*/

func (p *Player) GetDeviceID() (id string, found bool) {
	return p.DeviceID, p.KnownDeviceID
}

func (p *Player) setDeviceID(id string) {
	p.DeviceID = id
	p.KnownDeviceID = true
}

func (p *Player) GetEntityRuntimeID() (id uint64, found bool) {
	return p.EntityRuntimeID, p.KnownEntityRuntimeID
}

func (p *Player) setEntityRuntimeID(id uint64) {
	p.EntityRuntimeID = id
	p.KnownEntityRuntimeID = true
}

func (p *Player) GetEntityMetadata() (entityMetadata map[uint32]any, found bool) {
	return p.EntityMetadata, p.KnownEntityMetadata
}

func (p *Player) setEntityMetadata(entityMetadata map[uint32]any) {
	p.EntityMetadata = entityMetadata
	p.KnownEntityMetadata = true
}

func (p *Player) GetPosition() (position mgl32.Vec3, found bool) {
	return p.Position, p.KnownPosition
}

func (p *Player) setPosition(position mgl32.Vec3) {
	p.Position = position
	p.KnownPosition = true
}

func (p *Player) GetPitch() (pitch float32, found bool) {
	return p.Pitch, p.KnownPitch
}

func (p *Player) setPitch(pitch float32) {
	p.Pitch = pitch
	p.KnownPitch = true
}

func (p *Player) GetYaw() (yaw float32, found bool) {
	return p.Yaw, p.KnownYaw
}

func (p *Player) setYaw(yaw float32) {
	p.Yaw = yaw
	p.KnownYaw = true
}

func (p *Player) GetHeadYaw() (headYaw float32, found bool) {
    return p.HeadYaw, p.KnownHeadYaw
}

func (p *Player) setHeadYaw(headYaw float32) {
    p.HeadYaw = headYaw
    p.KnownHeadYaw = true
}

func (p *Player) GetMode() (mode byte, found bool) {
    return p.Mode, p.KnownMode
}

func (p *Player) setMode(mode byte) {
    p.Mode = mode
    p.KnownMode = true
}

func (p *Player) GetOnGround() (onGround bool, found bool) {
	return p.OnGround, p.KnownOnGround
}

func (p *Player) setOnGround(onGround bool) {
	p.OnGround = onGround
	p.KnownOnGround = true
}
func (p *Player) GetTick() (tick uint64, found bool) {
    return p.Tick, p.KnownTick
}

func (p *Player) setTick(tick uint64) {
    p.Tick = tick
    p.KnownTick = true
}

func (p *Player) GetRiddenEntityRuntimeID() (id uint64, found bool) {
    return p.RiddenEntityRuntimeID, p.KnownRiddenEntityRuntimeID
}

func (p *Player) setRiddenEntityRuntimeID(id uint64) {
    p.RiddenEntityRuntimeID = id
    p.KnownRiddenEntityRuntimeID = true
}

/*
func (p *Player) CanBuild() (hasAbility bool, found bool) {
	return p.CanBuildField, true
}
func (p *Player) CanMine() (hasAbility bool, found bool) {
	return p.CanMineField, true
}
func (p *Player) CanDoorsAndSwitches() (hasAbility bool, found bool) {
	return p.CanDoorsAndSwitchesField, true
}
func (p *Player) CanOpenContainers() (hasAbility bool, found bool) {
	return p.CanOpenContainersField, true
}
func (p *Player) CanAttackPlayers() (hasAbility bool, found bool) {
	return p.CanAttackPlayersField, true
}
func (p *Player) CanAttackMobs() (hasAbility bool, found bool) {
	return p.CanAttackMobsField, true
}
func (p *Player) CanOperatorCommands() (hasAbility bool, found bool) {
	return p.CanOperatorCommandsField, true
}
func (p *Player) CanTeleport() (hasAbility bool, found bool) {
	return p.CanTeleportField, true
}
func (p *Player) StatusInvulnerable() (hasStatus bool, found bool) {
	return p.StatusInvulnerableField, true
}
func (p *Player) StatusFlying() (hasStatus bool, found bool) {
	return p.StatusFlyingField, true
}
func (p *Player) StatusMayFly() (hasStatus bool, found bool) {
	return p.StatusMayFlyField, true
}
*/

// var AdventureFlagMap = map[string]uint32{
// 	"AdventureFlagWorldImmutable":        packet.AdventureFlagWorldImmutable,
// 	"AdventureSettingsFlagsNoPvM":        packet.AdventureSettingsFlagsNoPvM,
// 	"AdventureSettingsFlagsNoMvP":        packet.AdventureSettingsFlagsNoMvP,
// 	"AdventureSettingsFlagsUnused":       packet.AdventureSettingsFlagsUnused,
// 	"AdventureSettingsFlagsShowNameTags": packet.AdventureSettingsFlagsShowNameTags,
// 	"AdventureFlagAutoJump":              packet.AdventureFlagAutoJump,
// 	"AdventureFlagAllowFlight":           packet.AdventureFlagAllowFlight,
// 	"AdventureFlagNoClip":                packet.AdventureFlagNoClip,
// 	"AdventureFlagWorldBuilder":          packet.AdventureFlagWorldBuilder,
// 	"AdventureFlagFlying":                packet.AdventureFlagFlying,
// 	"AdventureFlagMuted":                 packet.AdventureFlagMuted,
// }

// var ActionPermissionMap = map[string]uint32{
// 	"ActionPermissionMine":             packet.ActionPermissionMine,
// 	"ActionPermissionDoorsAndSwitches": packet.ActionPermissionDoorsAndSwitches,
// 	"ActionPermissionOpenContainers":   packet.ActionPermissionOpenContainers,
// 	"ActionPermissionAttackPlayers":    packet.ActionPermissionAttackPlayers,
// 	"ActionPermissionAttackMobs":       packet.ActionPermissionAttackMobs,
// 	"ActionPermissionOperator":         packet.ActionPermissionOperator,
// 	"ActionPermissionTeleport":         packet.ActionPermissionTeleport,
// 	"ActionPermissionBuild":            packet.ActionPermissionBuild,
// 	"ActionPermissionDefault":          packet.ActionPermissionDefault,
// }

// func (p *Player) GetAbilityString() (adventureFlagsMap, actionPermissionMap map[string]bool, found bool) {
// 	adventureFlagsMap = make(map[string]bool)
// 	actionPermissionMap = make(map[string]bool)
// 	adventrueFlags, ok := p.GetPropertiesFlag()
// 	if !ok {
// 		return
// 	}
// 	actionFlags, ok := p.GetActionPermissions()
// 	if !ok {
// 		return
// 	}

// 	for flagName, flagValue := range AdventureFlagMap {
// 		adventureFlagsMap[flagName] = (adventrueFlags & flagValue) != 0
// 	}
// 	for flagName, flagValue := range ActionPermissionMap {
// 		actionPermissionMap[flagName] = (actionFlags & flagValue) != 0
// 	}

// 	return adventureFlagsMap, actionPermissionMap, true
// }
