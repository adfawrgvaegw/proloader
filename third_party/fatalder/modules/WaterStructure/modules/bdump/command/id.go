package command

const (
	IDCreateConstantString uint16 = 1
)

const (
	IDPlaceBlockWithBlockStates uint16 = 5 + iota
	IDAddInt16ZValue0
	IDPlaceBlock
	IDAddZValue0
	IDNoOperation
)

const (
	IDAddInt32ZValue0 uint16 = 12 + iota
	IDPlaceBlockWithBlockStatesDeprecated
	IDAddXValue
	IDSubtractXValue
	IDAddYValue
	IDSubtractYValue
	IDAddZValue
	IDSubtractZValue
)

const (
	IDAddInt16XValue uint16 = 20 + iota
	IDAddInt32XValue
	IDAddInt16YValue
	IDAddInt32YValue
	IDAddInt16ZValue
	IDAddInt32ZValue
	IDSetCommandBlockData
	IDPlaceBlockWithCommandBlockData
	IDAddInt8XValue
	IDAddInt8YValue
	IDAddInt8ZValue
	IDUseRuntimeIDPool
	IDPlaceRuntimeBlock
	IDPlaceRuntimeBlockWithUint32RuntimeID
	IDPlaceRuntimeBlockWithCommandBlockData
	IDPlaceRuntimeBlockWithCommandBlockDataAndUint32RuntimeID
	IDPlaceCommandBlockWithCommandBlockData
	IDPlaceRuntimeBlockWithChestData
	IDPlaceRuntimeBlockWithChestDataAndUint32RuntimeID
	IDAssignDebugData
	IDPlaceBlockWithChestData
	IDPlaceBlockWithNBTData
)

const (
	IDTerminate uint16 = 88
)
