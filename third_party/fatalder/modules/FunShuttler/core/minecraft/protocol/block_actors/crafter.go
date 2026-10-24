package block_actors

import general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"

// 合成器
type Crafter struct {
	general.DispenserBlockActor `mapstructure:",squash"`
	DisabledSlots               string `mapstructure:"disabled_slots"` // Not used; TAG_Short(3) = 0
}

// ID ...
func (*Crafter) ID() string {
	return IDCrafter
}
