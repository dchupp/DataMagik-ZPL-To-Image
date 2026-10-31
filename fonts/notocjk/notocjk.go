// Package notocjk embeds Noto Sans CJK SC Regular (SIL OFL 1.1) as a
// fallback font for Chinese, Japanese and Korean text. It is a separate
// package so that only programs which import it carry its ~16 MB.
//
//	opts := zpl.Options{FallbackFonts: [][]byte{notocjk.Regular}}
//
// The font contains the full Pan-CJK glyph set (Han, kana, Hangul) with
// Simplified Chinese glyph forms as the default.
package notocjk

import _ "embed"

// Regular is the OpenType (CFF) font data.
//
//go:embed NotoSansCJKsc-Regular.otf
var Regular []byte
