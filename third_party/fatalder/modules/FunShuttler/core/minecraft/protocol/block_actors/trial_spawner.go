package block_actors

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/fields"
	general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"
)

// 试炼刷怪笼
type TrialSpawner struct {
	general.BlockActor  `mapstructure:",squash"`
	NormalConfig        fields.TrialSpawnerConfig     `mapstructure:"normal_config"`                 // Not used; TAG_Compound(10)
	OminousConfig       fields.TrialSpawnerConfig     `mapstructure:"ominous_config"`                // Not used; TAG_Compound(10)
	SpawnData           *fields.TrialSpawnerSpawnData `mapstructure:"spawn_data,omitempty"`          // Not used; TAG_Compound(10)
	RequiredPlayerRange int32                         `mapstructure:"required_player_range"`         // Not used; TAG_Int(3) = 14
	RegisteredPlayers   []any                         `mapstructure:"registered_players"`            // Not used; TAG_List[TAG_Compound] (9[10]) = []
	CurrentMobs         []any                         `mapstructure:"current_mobs"`                  // Not used; TAG_List[TAG_Compound] (9[10]) = []
	NextMobSpawnsAt     int64                         `mapstructure:"next_mob_spawns_at"`            // Not used; TAG_Long(4) = 0
	CooldownEndAt       int64                         `mapstructure:"cooldown_end_at"`               // Not used; TAG_Long(4) = 0
	SelectedLootTable   *string                       `mapstructure:"selected_loot_table,omitempty"` // Not used; TAG_String(8)
}

// ID ...
func (*TrialSpawner) ID() string {
	return IDTrialSpawner
}

func (t *TrialSpawner) Marshal(io protocol.IO) {
	protocol.Single(io, &t.BlockActor)
}
