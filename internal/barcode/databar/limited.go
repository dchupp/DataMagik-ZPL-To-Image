package databar

// limitedWidths returns the 47 element widths of a DataBar Limited symbol,
// starting with the 1X light left-guard module and ending with the 5X light
// right guard.
func limitedWidths(gtin13 string, linkage bool) [47]int {
	val := atoi64(gtin13)
	if linkage {
		val += 2015133531096
	}
	pairs := [2]int{int(val / 2013571), int(val % 2013571)}
	var pw [2][]int
	for i, v := range pairs {
		g := 0
		for k := 6; k > 0; k-- {
			if v >= ltdGSum[k] {
				g = k
				break
			}
		}
		v -= ltdGSum[g]
		pw[i] = charWidths(v/ltdTEven[g], v%ltdTEven[g], ltdOddMod[g], 26-ltdOddMod[g], 7, ltdWidest[g], false)
	}
	sum := 0
	for i := range 14 {
		sum += ltdWeights[0][i]*pw[0][i] + ltdWeights[1][i]*pw[1][i]
	}
	check := ltdCheckPatterns[sum%89]

	var total [47]int
	total[0], total[1], total[44], total[45], total[46] = 1, 1, 1, 1, 5
	for i := range 14 {
		total[i+2] = pw[0][i]
		total[i+16] = int(check[i])
		total[i+30] = pw[1][i]
	}
	return total
}

func encodeLimited(data string, linkage bool) (Symbol, error) {
	gtin, err := parseGTIN(data)
	if err != nil {
		return Symbol{}, err
	}
	if gtin[0] > '1' {
		return Symbol{}, invalid("DataBar Limited GTIN-14 must start with 0 or 1, got %q", data)
	}
	total := limitedWidths(gtin, linkage)
	row := expand(total[:], false)
	s := Symbol{Text: gtinText(gtin)}
	s.Matrix.Rows = [][]bool{row}
	s.Matrix.RowHeights = []int{10}
	s.Linear, s.LeftOffset = toLinear(row, s.Text)
	if linkage {
		sep := make([]bool, len(row))
		for i := 4; i < 70; i++ {
			sep[i] = !row[i]
		}
		s.CompositeSeparator = sep
	}
	return s, nil
}
