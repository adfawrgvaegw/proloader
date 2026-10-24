package block_helper

// NearBlock 描述了各种 Helper 方块的相邻方块
type NearBlock struct {
	Name string
}

func (NearBlock) KnownBlockStates() bool {
	return true
}

func (n NearBlock) BlockName() string {
	return n.Name
}

func (NearBlock) BlockStates() map[string]any {
	return map[string]any{}
}

func (NearBlock) BlockStatesString() string {
	return "[]"
}
