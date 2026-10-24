package chat_connection

// 聊天相关的操作码，保持与原 MCP 逻辑一致。
const (
	OPGroupChat    uint16 = 5
	OPGroupFriend  uint16 = 6
	OPChatPrivate  uint16 = 1
	OPChatGroup    uint16 = 48
	OPFriendApply  uint16 = 2
	OPFriendDelete uint16 = 4
	OPFriendReply  uint16 = 5
	OPFriendList   uint16 = 7

	OPLoginOnOtherDevice uint16 = 259
	OPBanLogin           uint16 = 260
	OPBanChat            uint16 = 32784
	OPBanItem            uint16 = 32785

	OPFriendOnline      uint16 = 10000
	OPFriendOnline2     uint16 = 10021
	OPSetBlockDelete    uint16 = 10022
	OPPushFriendApply   uint16 = 10001
	OPPushFriendReply   uint16 = 10002
	OPPushFriendDelete  uint16 = 10003
	OPPushFriendList    uint16 = 10005
	OPPlayerLeaveRoom   uint16 = 10242
	OPAIRemainCount     uint16 = 10248
	OPCloseFriendOnline uint16 = 10027
)

// 分享类型常量，保留以便后续扩展。
const (
	ShareTypeText       uint16 = 0
	ShareTypeAudio      uint16 = 1
	ShareTypeLanGame    uint16 = 2
	ShareTypeServerGame uint16 = 3
	ShareTypeVoiceRoom  uint16 = 4
	ShareTypeNotice     uint16 = 5
	ShareTypeNews       uint16 = 6
	ShareTypeProfile    uint16 = 7
	ShareTypeResource   uint16 = 8
	ShareTypeCategory   uint16 = 9
	ShareTypeRentGame   uint16 = 10
	ShareTypeLobbyGame  uint16 = 11
	ShareTypeCPPLanGame uint16 = 12
	ShareTypeHomeland   uint16 = 13
	ShareTypeLaXin      uint16 = 14
	ShareTypeNetGame    uint16 = 15
	ShareTypeInvite     uint16 = 16
	ShareTypeClaim      uint16 = 102
	ShareTypeDonate     uint16 = 103
	ShareTypeActivity   uint16 = 200
)

// Composite command helpers.
const (
	CommandChatPrivate = (OPGroupChat << 8) | OPChatPrivate
	CommandChatGroup   = (OPGroupChat << 8) | OPChatGroup
)
