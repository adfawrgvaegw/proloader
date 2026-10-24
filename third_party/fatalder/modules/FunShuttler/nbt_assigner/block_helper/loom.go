package block_helper

type LoomBlockHelper struct{}

func (LoomBlockHelper) KnownBlockStates() bool {
	return true
}

func (LoomBlockHelper) BlockName() string {
	return "minecraft:loom"
}

func (LoomBlockHelper) BlockStates() map[string]any {
	return map[string]any{
		"direction": int32(0),
	}
}

func (LoomBlockHelper) BlockStatesString() string {
	return `["direction"=0]`
}
