package structure

type GangBanV2 struct {
	GangBanV1
}

func (g *GangBanV2) ID() uint8 {
	return IDGangBanV2
}

func (g *GangBanV2) Name() string {
	return NameGangBanV2
}
