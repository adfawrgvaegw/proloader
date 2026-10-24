package minecraft

import (
	mei "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/interface"
	"github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/server_to_client/minecraft/achievement"
)

// 成就
type Achievement struct{ mei.Module }

// Return the module name of a
func (a *Achievement) ModuleName() string {
	return "achievement"
}

// Return a pool/map that contains all the event of a
func (a *Achievement) EventPool() map[string]mei.Event {
	return map[string]mei.Event{
		"InitInformation": &achievement.InitInformation{},
	}
}
