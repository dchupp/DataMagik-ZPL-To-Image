package engine

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// serialSpec is ^SN (kind 'N') or ^SF (kind 'F').
type serialSpec struct {
	kind  byte
	start string // ^SN starting value
	inc   string // increment (^SN number, ^SF increment string)
	zeros bool   // ^SN leading zeros
	mask  string // ^SF mask
}

// fieldData returns a field's data after serialisation, ^FE insertion,
// ^FH hex escapes and ^FC clock substitution.
func (f *format) fieldData(fld *field, st fieldState) []byte {
	data := append([]byte(nil), fld.data...)
	idx := f.copyIndex / (f.replicates + 1)
	if s := fld.serial; s != nil {
		switch s.kind {
		case 'N':
			data = []byte(serialNumber(s.start, s.inc, s.zeros, idx))
		case 'F':
			data = []byte(serialMask(string(data), s.mask, s.inc, idx))
		}
	}
	if st.fePos != 0 {
		data = f.insertFN(data, st.fePos)
	}
	if fld.hexInd != 0 {
		data = decodeFH(data, fld.hexInd)
	}
	if fld.clock != nil {
		data = f.clockText(data, *fld.clock)
	}
	return data
}

// decodeFH replaces indicator+two hex digits with the byte value.
func decodeFH(data []byte, ind byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == ind && i+2 < len(data) {
			if v, err := strconv.ParseUint(string(data[i+1:i+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				i += 2
				continue
			}
		}
		out = append(out, data[i])
	}
	return out
}

// insertFN expands ^FE references: #n# and #n,f|b,x,y#.
func (f *format) insertFN(data []byte, ch byte) []byte {
	var out []byte
	for i := 0; i < len(data); i++ {
		if data[i] != ch {
			out = append(out, data[i])
			continue
		}
		end := strings.IndexByte(string(data[i+1:]), ch)
		if end < 0 {
			out = append(out, data[i:]...)
			break
		}
		ref := string(data[i+1 : i+1+end])
		if v, ok := f.fnRef(ref); ok {
			out = append(out, v...)
			i += end + 1
			continue
		}
		out = append(out, data[i])
	}
	return out
}

func (f *format) fnRef(ref string) ([]byte, bool) {
	parts := strings.Split(ref, ",")
	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, false
	}
	d, ok := f.fnData[n]
	if !ok {
		return nil, true
	}
	if len(parts) != 4 {
		return d, true
	}
	x, err1 := strconv.Atoi(strings.TrimSpace(parts[2]))
	y, err2 := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err1 != nil || err2 != nil || x <= 0 || y < 0 {
		return nil, true
	}
	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "f":
		start := x - 1
		if start >= len(d) {
			return nil, true
		}
		return d[start:min(start+y, len(d))], true
	case "b":
		// Counting from the end: x=1 is the last character, taking y
		// characters ending there.
		endIdx := len(d) - x + 1
		if endIdx <= 0 {
			return nil, true
		}
		return d[max(endIdx-y, 0):endIdx], true
	}
	return nil, true
}

// serialNumber computes the ^SN value for label idx.
func serialNumber(start, inc string, zeros bool, idx int) string {
	// The indexed portion is the right-most run of digits.
	end := len(start)
	for end > 0 && (start[end-1] < '0' || start[end-1] > '9') {
		end--
	}
	if end == 0 {
		return start
	}
	begin := end
	for begin > 0 && start[begin-1] >= '0' && start[begin-1] <= '9' {
		begin--
	}
	// Serial numbers hold at most 12 digits; indexing only the right-most
	// 18 keeps the arithmetic within int64 for any input.
	begin = max(begin, end-18)
	// Leading spaces belong to the field width.
	padStart := begin
	for padStart > 0 && start[padStart-1] == ' ' {
		padStart--
	}
	width := end - padStart
	v, _ := strconv.ParseInt(start[begin:end], 10, 64)
	n, err := strconv.ParseInt(strings.TrimSpace(inc), 10, 64)
	if err != nil {
		n = 1
	}
	v += n * int64(idx)
	mod := int64(1)
	for range end - begin {
		mod *= 10
	}
	v = ((v % mod) + mod) % mod
	var num string
	if zeros {
		num = fmt.Sprintf("%0*d", width, v)
	} else {
		num = fmt.Sprintf("%*d", width, v)
	}
	return start[:padStart] + num + start[end:]
}

// serialMask applies ^SF serialisation to data for label idx.
func serialMask(data, mask, inc string, idx int) string {
	if idx == 0 || mask == "" {
		return data
	}
	rs := []rune(data)
	ms := []rune(mask)
	is := []rune(inc)
	carry := 0
	for k := 1; k <= len(ms) && k <= len(rs); k++ {
		m := ms[len(ms)-k]
		pos := len(rs) - k
		radix, val, ok := maskDigit(m, rs[pos])
		if !ok {
			continue
		}
		incVal := 0
		if k <= len(is) {
			_, iv, iok := maskDigit(m, is[len(is)-k])
			if iok {
				incVal = iv
			}
		} else if k == 1 && inc == "" {
			incVal = 1
		}
		total := val + incVal*idx + carry
		rs[pos] = maskRune(m, total%radix, rs[pos])
		carry = total / radix
	}
	return string(rs)
}

const alnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func maskDigit(m, r rune) (radix, val int, ok bool) {
	switch m {
	case 'D', 'd':
		return 10, digitVal(r, 10), true
	case 'H', 'h':
		return 16, digitVal(r, 16), true
	case 'O', 'o':
		return 8, digitVal(r, 8), true
	case 'A', 'a':
		u := r &^ 0x20
		if u >= 'A' && u <= 'Z' {
			return 26, int(u - 'A'), true
		}
		return 26, 0, true
	case 'N', 'n':
		u := r
		if u >= 'a' && u <= 'z' {
			u -= 0x20
		}
		if i := strings.IndexRune(alnum, u); i >= 0 {
			return 36, i, true
		}
		return 36, 0, true
	}
	return 0, 0, false
}

func digitVal(r rune, radix int) int {
	v, err := strconv.ParseInt(string(r), radix, 8)
	if err != nil {
		return 0
	}
	return int(v)
}

func maskRune(m rune, v int, orig rune) rune {
	lower := orig >= 'a' && orig <= 'z'
	var r rune
	switch m {
	case 'D', 'd', 'O', 'o':
		r = rune('0' + v)
	case 'H', 'h':
		r = rune(alnum[v])
	case 'A', 'a':
		r = rune('A' + v)
	case 'N', 'n':
		r = rune(alnum[v])
	}
	if lower && r >= 'A' && r <= 'Z' {
		r += 0x20
	}
	return r
}

// now returns the printer clock, offset per indicator (0 primary, 1
// secondary, 2 third).
func (f *format) now(which int) time.Time {
	t := f.pr.opt.Now().Add(f.pr.p.clockOff)
	if which > 0 {
		o := f.pr.p.clockOffs[which+1]
		t = t.AddDate(o.years, o.months, o.days)
		t = t.Add(time.Duration(o.hours)*time.Hour + time.Duration(o.minutes)*time.Minute + time.Duration(o.seconds)*time.Second)
	}
	return t
}

// clockText substitutes real-time clock codes after each indicator.
func (f *format) clockText(data []byte, ind [3]byte) []byte {
	var out []byte
	for i := 0; i < len(data); i++ {
		which := -1
		for k, c := range ind {
			if c != 0 && data[i] == c {
				which = k
			}
		}
		if which < 0 || i+1 >= len(data) {
			out = append(out, data[i])
			continue
		}
		s, ok := clockCode(data[i+1], f.now(which))
		if !ok {
			out = append(out, data[i])
			continue
		}
		out = append(out, s...)
		i++
	}
	return out
}

// clockCode formats one Zebra real-time clock code.
func clockCode(c byte, t time.Time) (string, bool) {
	switch c {
	case 'a':
		return t.Format("Mon"), true
	case 'A':
		return t.Format("Monday"), true
	case 'b':
		return t.Format("Jan"), true
	case 'B':
		return t.Format("January"), true
	case 'd':
		return t.Format("02"), true
	case 'H':
		return t.Format("15"), true
	case 'I':
		return t.Format("03"), true
	case 'j':
		return fmt.Sprintf("%03d", t.YearDay()), true
	case 'm':
		return t.Format("01"), true
	case 'M':
		return t.Format("04"), true
	case 'p':
		return t.Format("PM"), true
	case 'S':
		return t.Format("05"), true
	case 'y':
		return t.Format("06"), true
	case 'Y':
		return t.Format("2006"), true
	case 'w':
		return strconv.Itoa(int(t.Weekday())), true
	case 'U':
		return fmt.Sprintf("%02d", (t.YearDay()+6-int(t.Weekday()))/7), true
	case 'W':
		wd := (int(t.Weekday()) + 6) % 7
		return fmt.Sprintf("%02d", (t.YearDay()+6-wd)/7), true
	case '%':
		return "%", true
	}
	return "", false
}
