package databar

// omniGroup returns the Table 1/2 group of a character value.
func omniGroup(v int, outside bool) int {
	if outside {
		for g := range 4 {
			if v < omnGSum[g+1] {
				return g
			}
		}
		return 4
	}
	for g := 5; g < 8; g++ {
		if v < omnGSum[g+1] {
			return g
		}
	}
	return 8
}

// omniWidths returns the 46 element widths of a DataBar Omnidirectional
// symbol (starting with the light left-guard module) and the right check
// value (needed by the Stacked Omni separator).
func omniWidths(gtin13 string, linkage bool) (total [46]int, cRight int) {
	val := atoi64(gtin13)
	if linkage {
		val += 10000000000000
	}
	left, right := int(val/4537077), int(val%4537077)
	chars := [4]int{left / 1597, left % 1597, right / 1597, right % 1597}

	var dw [4][]int
	for i, c := range chars {
		outside := i%2 == 0
		g := omniGroup(c, outside)
		v := c - omnGSum[g]
		div, mod := v/omnT[g], v%omnT[g]
		if outside {
			dw[i] = charWidths(div, mod, omnOddMod[g], omnEvenMod[g], 4, omnWidest[g], false)
		} else {
			dw[i] = charWidths(mod, div, omnOddMod[g], omnEvenMod[g], 4, omnWidest[g], true)
		}
	}

	sum := 0
	for i := range 4 {
		for j := range 8 {
			sum += omnWeights[i][j] * dw[i][j]
		}
	}
	sum %= 79
	if sum >= 8 {
		sum++
	}
	if sum >= 72 {
		sum++
	}
	cLeft, cRight := sum/9, sum%9

	total[0], total[1], total[44], total[45] = 1, 1, 1, 1
	for i := range 8 {
		total[i+2] = dw[0][i]
		total[i+15] = dw[1][7-i]
		total[i+23] = dw[3][i]
		total[i+36] = dw[2][7-i]
	}
	for i := range 5 {
		total[i+10] = omnFinder[cLeft][i]
		total[i+31] = omnFinder[cRight][4-i]
	}
	return total, cRight
}

// omniSeparator builds a separator row for the given module row
// (ISO/IEC 24724 5.3.2.2 / composite separator): the complement of the row
// with 4 light modules at each end, and the 13 modules over each finder
// (starting at the given offsets) alternating light/dark over its spaces.
// When finderValue3 is set the bottom-row rule for finder value 3 applies.
func omniSeparator(row []bool, finders []int, finderValue3 bool) []bool {
	sep := make([]bool, len(row))
	for i := 4; i < len(row)-4; i++ {
		sep[i] = !row[i]
	}
	for _, f := range finders {
		if finderValue3 {
			// 5.3.2.2: the single dark module over finder value 3 is shifted
			// right to sit over the start of the 3X finder bar.
			for i := f; i < f+13; i++ {
				sep[i] = i == f+10
			}
			continue
		}
		latch := true
		for i := f; i < f+13; i++ {
			if !row[i] {
				sep[i] = latch
				latch = !latch
			} else {
				sep[i] = false
				latch = true
			}
		}
	}
	return sep
}

func encodeOmni(t Type, data string, linkage bool) (Symbol, error) {
	gtin, err := parseGTIN(data)
	if err != nil {
		return Symbol{}, err
	}
	total, cRight := omniWidths(gtin, linkage)
	s := Symbol{Text: gtinText(gtin)}

	switch t {
	case TypeOmnidirectional, TypeTruncated:
		row := expand(total[:], false)
		h := 33
		if t == TypeTruncated {
			h = 13
		}
		s.Matrix.Rows = [][]bool{row}
		s.Matrix.RowHeights = []int{h}
		s.Linear, s.LeftOffset = toLinear(row, s.Text)
		if linkage {
			s.CompositeSeparator = omniSeparator(row, []int{18, 63}, false)
		}
		return s, nil
	}

	// Two-row variants: top = left guard, chars 1 & 2, right guard (bar,
	// space); bottom = left guard (bar, space), chars 4 & 3, right guard.
	top := append(expand(total[:23], false), true, false)
	bottom := append([]bool{true, false}, expand(total[23:], true)...)

	if t == TypeStacked {
		// 5.3.2.1 separator: light where rows differ unless the previous
		// separator module is light (alternation), dark where both light.
		sep := make([]bool, 50)
		for i := 1; i < 46; i++ {
			if top[i] == bottom[i] {
				sep[i] = !top[i]
			} else {
				sep[i] = !sep[i-1]
			}
		}
		sep[1], sep[2], sep[3] = false, false, false
		s.Matrix.Rows = [][]bool{top, sep, bottom}
		s.Matrix.RowHeights = []int{5, 1, 7}
		s.SeparatorRows = []int{1}
	} else {
		mid := make([]bool, 50)
		for i := 5; i < 46; i += 2 {
			mid[i] = true
		}
		sepTop := omniSeparator(top, []int{18}, false)
		// 17 = guard + inner char; +2 skips finder elements 4 and 5, which
		// read right to left in the bottom row.
		sepBottom := omniSeparator(bottom, []int{19}, cRight == 3)
		s.Matrix.Rows = [][]bool{top, sepTop, mid, sepBottom, bottom}
		s.Matrix.RowHeights = []int{33, 1, 1, 1, 33}
		s.SeparatorRows = []int{1, 2, 3}
	}
	if linkage {
		s.CompositeSeparator = omniSeparator(top, []int{18}, false)
	}
	return s, nil
}
