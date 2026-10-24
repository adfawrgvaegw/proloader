package minecraft

import (
	mei "github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/interface"
	"github.com/Yeah114/FunShuttler/core/py_rpc/mod_event/server_to_client/minecraft/chat_phrases"
)

// 快捷游戏短语
type ChatPhrases struct{ mei.Module }

// Return the module name of c
func (c *ChatPhrases) ModuleName() string {
	return "chatPhrases"
}

// Return a pool/map that contains all the event of c
func (c *ChatPhrases) EventPool() map[string]mei.Event {
	return map[string]mei.Event{
		"SyncNewPlayerPhrasesData": &chat_phrases.SyncNewPlayerPhrasesData{},
	}
}
