package structure

type MianYangV2 struct {
	MianYangV1
}

func (m *MianYangV2) ID() uint8 {
	return IDMianYangV2
}

func (m *MianYangV2) Name() string {
	return NameMianYangV2
}
