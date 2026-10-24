package block_helper

import "github.com/Yeah114/FunShuttler/utils"

// ComplexBlock 描述一种复杂的方块，它除了具有名称和方块状态以外，
// 它还具有很多其他的数据。为了方便，此处不会记录额外的数据
type ComplexBlock struct {
	KnownStates bool
	Name        string
	States      map[string]any
}

func (c ComplexBlock) KnownBlockStates() bool {
	return c.KnownStates
}

func (c ComplexBlock) BlockName() string {
	return c.Name
}

func (c ComplexBlock) BlockStates() map[string]any {
	return c.States
}

func (c ComplexBlock) BlockStatesString() string {
	return utils.MarshalBlockStates(c.States)
}
