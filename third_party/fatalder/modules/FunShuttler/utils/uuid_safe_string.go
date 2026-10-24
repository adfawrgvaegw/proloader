package utils

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// StringUUIDReplaceMap 保持一个字符串映射，
// 用于将字符串型的 UUID 转化为不包含屏蔽词
// 的安全字符串
var StringUUIDReplaceMap = map[rune]rune{
	'0': '+', '1': '−', '2': '|', '3': '–', '4': '×',
	'5': '÷', '6': '¦', '7': '—', '8': '=', '9': '(',

	'a': '<', 'b': '>', 'c': '⁅', 'd': '⁆', 'e': '[',
	'f': ']', 'g': '‹', 'h': '›', 'i': '⌈', 'j': '⌉',
	'k': '{', 'l': '}', 'm': '«', 'n': '»', 'o': '⌊',
	'p': '⌋', 'q': '⟨', 'r': '⟩', 's': '⟦', 't': '⟧',
	'u': '`', 'v': '´', 'w': '⟪', 'x': '⟫', 'y': '⟬',
	'z': '⟭',

	'-': '→',
}

// StringUUIDInvReplaceMap 是 StringUUIDReplaceMap 的逆映射，
// 并且它是在运行时生成的，因此无需手动重复上面的映射
var StringUUIDInvReplaceMap map[rune]rune

func init() {
	StringUUIDInvReplaceMap = make(map[rune]rune)
	for key, value := range StringUUIDReplaceMap {
		StringUUIDInvReplaceMap[value] = key
	}
}

// MakeUUIDSafeString 返回 uniqueID 的安全化表示，
// 这使得其不可能被网易屏蔽词所拦截
func MakeUUIDSafeString(uniqueID uuid.UUID) string {
	var builder strings.Builder
	for _, value := range uniqueID.String() {
		builder.WriteRune(StringUUIDReplaceMap[value])
	}
	return builder.String()
}

// FromUUIDSafeString 将一个安全化表示的 uuidSafeString
// 重新转换回它的通常 UUID 表示
func FromUUIDSafeString(uuidSafeString string) (result uuid.UUID, err error) {
	var builder strings.Builder
	for _, value := range uuidSafeString {
		builder.WriteRune(StringUUIDInvReplaceMap[value])
	}
	result, err = uuid.Parse(builder.String())
	if err != nil {
		return result, fmt.Errorf("FromUUIDSafeString: %v", err)
	}
	return
}
