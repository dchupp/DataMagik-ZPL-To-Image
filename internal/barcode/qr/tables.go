package qr

// Error correction tables, indexed [level][version]. Level order is L, M, Q, H
// (the ECLevel constants). Index 0 is unused.

// Model 2, ISO/IEC 18004:2015 Table 9.
var ecCodewordsPerBlock2 = [4][41]int{
	{0, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	{0, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{0, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

var numBlocks2 = [4][41]int{
	{0, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	{0, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	{0, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}

// Model 1 (ISO/IEC 18004:2000 Annex M, Table M.2): every version uses
// equally sized blocks. Values are {ec codewords per block, block count, data codewords per block}.
var model1Blocks = [15][4][3]int{
	{},
	{{7, 1, 19}, {10, 1, 16}, {13, 1, 13}, {17, 1, 9}},
	{{10, 1, 36}, {16, 1, 30}, {22, 1, 24}, {30, 1, 16}},
	{{15, 1, 57}, {28, 1, 44}, {36, 1, 36}, {48, 1, 24}},
	{{20, 1, 80}, {40, 1, 60}, {50, 1, 50}, {66, 1, 34}},
	{{26, 1, 108}, {52, 1, 82}, {66, 1, 68}, {44, 2, 23}},
	{{34, 1, 136}, {32, 2, 53}, {42, 2, 43}, {56, 2, 29}},
	{{42, 1, 170}, {40, 2, 66}, {52, 2, 54}, {46, 3, 24}},
	{{24, 2, 104}, {48, 2, 80}, {64, 2, 64}, {56, 3, 29}},
	{{30, 2, 123}, {60, 2, 93}, {50, 3, 52}, {68, 3, 34}},
	{{34, 2, 145}, {68, 2, 111}, {58, 3, 61}, {58, 4, 31}},
	{{40, 2, 168}, {40, 4, 64}, {52, 4, 52}, {54, 5, 29}},
	{{46, 2, 192}, {46, 4, 73}, {58, 4, 61}, {62, 5, 33}},
	{{36, 3, 144}, {52, 4, 83}, {66, 4, 69}, {58, 6, 32}},
	{{40, 3, 163}, {60, 4, 92}, {60, 5, 62}, {66, 6, 35}},
}

// model1TotalCodewords is the number of 8-module codeword positions in a
// Model 1 symbol; positions beyond the blocks hold zero remainder codewords.
var model1TotalCodewords = [15]int{0, 26, 46, 72, 100, 134, 170, 212, 256, 306, 358, 416, 476, 542, 610}

// totalCodewords returns the number of codewords (data + EC) of a symbol.
func totalCodewords(model, version int) int {
	if model == 1 {
		return model1TotalCodewords[version]
	}
	ver := version
	n := (16*ver+128)*ver + 64
	if ver >= 2 {
		na := ver/7 + 2
		n -= (25*na-10)*na - 55
		if ver >= 7 {
			n -= 36
		}
	}
	return n / 8
}

// blockLayout returns the EC codewords per block and the number of blocks.
func blockLayout(model, version int, level ECLevel) (ecPerBlock, blocks int) {
	if model == 1 {
		b := model1Blocks[version][level]
		return b[0], b[1]
	}
	return ecCodewordsPerBlock2[level][version], numBlocks2[level][version]
}

// dataCodewords returns the number of data codewords of a symbol.
func dataCodewords(model, version int, level ECLevel) int {
	if model == 1 {
		b := model1Blocks[version][level]
		return b[1] * b[2]
	}
	ec, nb := blockLayout(model, version, level)
	return totalCodewords(model, version) - ec*nb
}

// alignmentPositions returns the alignment pattern centre coordinates for a
// Model 2 version.
func alignmentPositions(version int) []int {
	if version == 1 {
		return nil
	}
	n := version/7 + 2
	step := (version*8 + n*3 + 5) / (n*4 - 4) * 2
	size := version*4 + 17
	pos := make([]int, n)
	pos[0] = 6
	for i, p := n-1, size-7; i >= 1; i, p = i-1, p-step {
		pos[i] = p
	}
	return pos
}

// charCountBits is the character count indicator length (ISO/IEC 18004
// Table 3). The same lengths apply to Model 1 versions 1-14.
func charCountBits(m Mode, version int) int {
	c := 0
	switch {
	case version >= 27:
		c = 2
	case version >= 10:
		c = 1
	}
	switch m {
	case ModeNumeric:
		return [3]int{10, 12, 14}[c]
	case ModeAlphanumeric:
		return [3]int{9, 11, 13}[c]
	case ModeByte:
		return [3]int{8, 16, 16}[c]
	case ModeKanji:
		return [3]int{8, 10, 12}[c]
	}
	return 0
}
