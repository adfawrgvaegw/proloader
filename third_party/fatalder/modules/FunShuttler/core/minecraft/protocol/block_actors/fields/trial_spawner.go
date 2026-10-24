package fields

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// ------------------------- SpawnData -------------------------

// 描述 试炼刷怪笼 中的一个复用字段
type TrialSpawnerSpawnData struct {
	TypeID             string `mapstructure:"TypeId"`                         // Not used; TAG_String(8)
	Weight             int32  `mapstructure:"Weight"`                         // Not used; TAG_Int(3) = 1
	EquipmentLootTable string `mapstructure:"equipment_loot_table,omitempty"` // Not used; TAG_String(8)
}

func (t *TrialSpawnerSpawnData) Marshal(r protocol.IO) {}

// ------------------------- Config -------------------------

// 描述 试炼刷怪笼 中的一个复用字段
type TrialSpawnerConfig struct {
	SimultaneousMobs               float32 `mapstructure:"simultaneous_mobs"`                  // Not used; TAG_Float(5) = 2
	TotalMobs                      float32 `mapstructure:"total_mobs"`                         // Not used; TAG_Float(5) = 6
	TotalMobsAddedPerPlayer        float32 `mapstructure:"total_mobs_added_per_player"`        // Not used; TAG_Float(5) = 2
	SimultaneousMobsAddedPerPlayer float32 `mapstructure:"simultaneous_mobs_added_per_player"` // Not used; TAG_Float(5) = 1
	TargetCooldownLength           int32   `mapstructure:"target_cooldown_length"`             // Not used; TAG_Int(3) = 36000
	TicksBetweenSpawn              int32   `mapstructure:"ticks_between_spawn"`                // Not used; TAG_Int(3) = 20
	SpawnRange                     int32   `mapstructure:"spawn_range"`                        // Not used; TAG_Int(3) = 4
	LootTablesToEject              []any   `mapstructure:"loot_tables_to_eject"`               // Not used; TAG_List[TAG_Compound] (9[10])
	SpawnPotentials                []any   `mapstructure:"spawn_potentials"`                   // Not used; TAG_List[TAG_Compound] (9[10])
	ItemsToDropWhenOminous         string  `mapstructure:"items_to_drop_when_ominous"`         // Not used; TAG_String (8) = "loot_tables/spawners/trial_chamber/items_to_drop_when_ominous.json"
}

func (t *TrialSpawnerConfig) Marshal(r protocol.IO) {}

// ------------------------- End -------------------------
