package nbt_parser_general

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/mapping"
)

// 描述旗帜的种类
const (
	BannerTypeNormal  int32 = iota // 普通旗帜
	BannerTypeOminous              // 不祥旗帜
)

// BannerBaseColorDefault 是旗帜的默认颜色 (黑色)
const BannerBaseColorDefault int32 = iota

// BannerPattern 是旗帜的单个图案
type BannerPattern struct {
	Color   int32  `mapstructure:"Color"`
	Pattern string `mapstructure:"Pattern"`
}

// Format ..
func (b BannerPattern) Format(prefix string) string {
	if b.Pattern == mapping.BannerPatternOminous {
		return prefix + "不祥\n"
	}
	colorName := mapping.ColorFormat[b.Color]
	patternName := mapping.BannerPatternFormat[b.Pattern]
	return prefix + colorName + patternName + "\n"
}

// Marshal ..
func (b *BannerPattern) Marshal(io protocol.IO) {
	io.Varint32(&b.Color)
	io.String(&b.Pattern)
}
