package block_helper

// Air 描述了一个空气方块
type Air struct{}

func (Air) KnownBlockStates() bool {
	return true
}

func (Air) BlockName() string {
	return "minecraft:air"
}

func (Air) BlockStates() map[string]any {
	return map[string]any{}
}

func (Air) BlockStatesString() string {
	return `[]`
}
