package engine

import (
	"image"
	"strings"
	"time"

	"github.com/datamagik/zpl-to-image/internal/bitmap"
	"github.com/datamagik/zpl-to-image/internal/compress"
	"github.com/datamagik/zpl-to-image/internal/encoding"
	"github.com/datamagik/zpl-to-image/internal/lexer"
)

type handler func(f *format, c lexer.Command, ps params) error

// handlers maps every ZPL II command to its implementation. Commands that
// only affect printer hardware, communications or host status are accepted
// as no-ops, since they have no effect on the printed image.
var handlers map[string]handler

func noop(*format, lexer.Command, params) error { return nil }

// NoOpCommands lists commands that are accepted without affecting the
// image. It is exported for the coverage test.
var NoOpCommands = []string{
	// Format control handled elsewhere (lexer, Run, runFormat).
	"^XA", "^XZ", "^CC", "~CC", "^CT", "~CT", "^CD", "~CD", "^PQ", "^FX",
	// Media, print mechanism and calibration.
	"^MA", "^MD", "~SD", "^MF", "^MI", "^ML", "^MM", "^MN", "^MP", "^MT", "^MU", "^MW",
	"^PF", "^PH", "~PH", "~PL", "^PN", "^PP", "~PP", "^PR", "~PR", "~PS", "~PM", "~RO", "~TA",
	"^XB", "^XS", "^ZZ", "^CN", "^CO", "^CP", "^SC", "^SS", "^SI", "^SL", "^SP", "^SQ", "^SR", "^SX", "^SZ",
	"^JB", "~JB", "~JA", "~JC", "~JD", "~JE", "~JF", "~JG", "^JH", "^JI", "~JI", "^JJ", "~JL", "~JN",
	"~JO", "~JP", "~JQ", "~JR", "^JS", "~JS", "^JT", "^JU", "^JW", "~JX", "^JZ",
	"~KB", "^KC", "^KD", "^KL", "^KN", "^KP", "^KV", "^LF",
	// Host status, upload and diagnostics.
	"~HB", "~HD", "^HF", "^HG", "^HH", "~HI", "~HM", "~HQ", "~HS", "^HT", "~HU", "^HV", "^HW",
	"^HY", "^HZ", "~HL", "^HL", "~WC", "^WD", "~WQ", "~WL", "~WR", "~WS",
	// Networking and wireless.
	"^NB", "^NC", "~NC", "^ND", "^NI", "^NN", "^NP", "~NR", "^NS", "^NT", "~NT", "^NW",
	"^WA", "^WE", "^WF", "^WI", "^WL", "^WN", "^WP", "^WR", "^WS", "^WT", "^WX",
	// RFID (encoding a tag does not change the printed image).
	"^HR", "^RA", "^RB", "^RE", "^RF", "^RI", "^RL", "^RM", "^RN", "^RQ", "^RR", "^RS", "^RT",
	"^RU", "~RV", "^RW", "^RZ",
	// Download abort.
	"~DN",
}

func init() {
	handlers = map[string]handler{}
	for _, k := range NoOpCommands {
		handlers[k] = noop
	}
	for k, h := range map[string]handler{
		"^FO": cmdFO, "^FT": cmdFT, "^FD": cmdFD, "^FV": cmdFV, "^FS": cmdFS,
		"^FH": cmdFH, "^FB": cmdFB, "^TB": cmdTB, "^FP": cmdFP, "^FR": cmdFR,
		"^FW": cmdFW, "^FN": cmdFN, "^FC": cmdFC, "^FE": cmdFE, "^FL": cmdFL,
		"^FM": cmdFM, "^A": cmdA, "^CF": cmdCF, "^CW": cmdCW, "^CI": cmdCI,
		"^CV": cmdCV, "^CM": cmdCM, "^BY": cmdBY, "^PA": cmdPA, "^SE": cmdSE,
		"^GB": cmdGB, "^GC": cmdGC, "^GD": cmdGD, "^GE": cmdGE, "^GF": cmdGF,
		"^GS": cmdGS, "^XG": cmdXG, "^IM": cmdIM, "^IL": cmdIL, "^IS": cmdIS,
		"^ID": cmdID, "~EG": cmdEG, "^EG": cmdEG, "^DF": cmdDF, "^XF": cmdXF,
		"^TO": cmdTO, "^LH": cmdLH, "^LL": cmdLL, "^PW": cmdPW, "^LS": cmdLS,
		"^LT": cmdLT, "^LR": cmdLR, "^PO": cmdPO, "^PM": cmdPM, "^JM": cmdJM,
		"^MC": cmdMC, "^SN": cmdSN, "^SF": cmdSF, "^SO": cmdSO, "^ST": cmdST,
		"~DG": cmdDG, "~DY": cmdDY, "~DB": cmdDB, "~DU": cmdDU, "~DT": cmdDT,
		"~DS": cmdDS, "~DE": cmdDE,
	} {
		handlers[k] = h
	}
	for _, b := range BarcodeCommands {
		handlers[b] = cmdBarcode
	}
}

// Field positioning.

func cmdFO(f *format, _ lexer.Command, ps params) error {
	if f.fld.originSet && !f.fld.empty() {
		f.endField()
	}
	f.fld.originSet, f.fld.typeset, f.fld.ftNoPos = true, false, false
	f.fld.x = ps.int(0, 0) + f.pr.p.lhX
	f.fld.y = ps.int(1, 0) + f.pr.p.lhY
	if ps.has(2) {
		f.fld.just, f.fld.justSet = clamp(ps.int(2, 0), 0, 2), true
	}
	return nil
}

func cmdFT(f *format, _ lexer.Command, ps params) error {
	if f.fld.originSet && !f.fld.empty() {
		f.endField()
	}
	f.fld.originSet, f.fld.typeset = true, true
	f.fld.ftNoPos = !ps.has(0) && !ps.has(1)
	x, y := f.nextFT.X, f.nextFT.Y
	if ps.has(0) {
		x = ps.int(0, 0) + f.pr.p.lhX
	}
	if ps.has(1) {
		y = ps.int(1, 0) + f.pr.p.lhY
	}
	if !f.hasFT && f.fld.ftNoPos {
		x, y = f.pr.p.lhX, f.pr.p.lhY
	}
	f.fld.x, f.fld.y = x, y
	if ps.has(2) {
		f.fld.just, f.fld.justSet = clamp(ps.int(2, 0), 0, 2), true
	}
	return nil
}

func cmdFD(f *format, c lexer.Command, _ params) error {
	f.fld.data = append([]byte(nil), c.Params...)
	f.fld.hasData = true
	return nil
}

func cmdFV(f *format, c lexer.Command, ps params) error {
	_ = cmdFD(f, c, ps)
	f.fld.variable = true
	return nil
}

func cmdFS(f *format, _ lexer.Command, _ params) error {
	f.endField()
	return nil
}

func cmdFH(f *format, c lexer.Command, _ params) error {
	f.fld.hexInd = '_'
	if len(c.Params) > 0 && c.Params[0] != c.Delim {
		f.fld.hexInd = c.Params[0]
	}
	return nil
}

func cmdFB(f *format, _ lexer.Command, ps params) error {
	f.fld.block = &blockSpec{
		width:   clamp(ps.int(0, 0), 0, f.maxDim()),
		lines:   clamp(ps.int(1, 1), 1, 9999),
		spacing: clamp(ps.int(2, 0), -9999, f.maxDim()),
		just:    ps.char(3, 'L'),
		indent:  ps.int(4, 0),
	}
	return nil
}

func cmdTB(f *format, _ lexer.Command, ps params) error {
	tb := &tbSpec{width: clamp(ps.int(1, 1), 1, f.maxDim()), height: clamp(ps.int(2, 1), 1, f.maxDim())}
	tb.rot, tb.rotSet = parseRotation(ps.char(0, 0))
	f.fld.tb = tb
	return nil
}

func cmdFP(f *format, _ lexer.Command, ps params) error {
	d := ps.char(0, 'H')
	if d != 'V' && d != 'R' {
		d = 'H'
	}
	f.fld.dir, f.fld.gap = d, max(ps.int(1, 0), 0)
	return nil
}

func cmdFR(f *format, _ lexer.Command, _ params) error {
	f.fld.reverse = true
	return nil
}

func cmdFW(f *format, _ lexer.Command, ps params) error {
	if r, ok := parseRotation(ps.char(0, 0)); ok {
		f.st.fw.rot = r
	}
	if ps.has(1) {
		f.st.fw.just = clamp(ps.int(1, 0), 0, 2)
	}
	return nil
}

func cmdFN(f *format, c lexer.Command, _ params) error {
	n, _ := parseInt(strings.TrimSpace(string(c.Params)))
	f.fld.fn, f.fld.hasFN = n, true
	return nil
}

func cmdFC(f *format, _ lexer.Command, ps params) error {
	ind := [3]byte{'%', 0, 0}
	for i := range 3 {
		if s := ps.raw(i); s != "" {
			ind[i] = s[0]
		}
	}
	f.fld.clock = &ind
	return nil
}

func cmdFE(f *format, c lexer.Command, _ params) error {
	f.st.fePos = '#'
	if len(c.Params) > 0 {
		f.st.fePos = c.Params[0]
	}
	return nil
}

func cmdFL(f *format, _ lexer.Command, ps params) error {
	ext, base := ps.str(0), ps.str(1)
	if ext == "" || base == "" {
		return nil
	}
	key := f.pr.st.normalize(base, "")
	links := f.pr.p.links[key]
	out := links[:0]
	for _, l := range links {
		if !strings.EqualFold(l, ext) {
			out = append(out, l)
		}
	}
	if ps.int(2, -1) == 1 && len(out) < 5 {
		out = append(out, ext)
	}
	f.pr.p.links[key] = out
	return nil
}

func cmdFM(f *format, _ lexer.Command, ps params) error {
	f.fld.fm, f.fld.fmSkip = nil, nil
	for i := 0; i+1 < len(ps); i += 2 {
		skip := strings.EqualFold(ps.str(i), "e") || strings.EqualFold(ps.str(i+1), "e")
		f.fld.fm = append(f.fld.fm, image.Pt(ps.int(i, 0)+f.pr.p.lhX, ps.int(i+1, 0)+f.pr.p.lhY))
		f.fld.fmSkip = append(f.fld.fmSkip, skip)
	}
	return nil
}

// Fonts and encodings.

func cmdA(f *format, c lexer.Command, _ params) error {
	if len(c.Params) == 0 {
		return nil
	}
	name := upperByte(c.Params[0])
	rest := params(strings.Split(string(c.Params[1:]), string(c.Delim)))
	sel := &fontSel{name: name}
	sel.rot, sel.rotSet = parseRotation(rest.char(0, 0))
	sel.h, sel.w = clamp(rest.int(1, 0), 0, f.maxDim()), clamp(rest.int(2, 0), 0, f.maxDim())
	if name == '@' {
		sel.name = 0
		sel.path = rest.str(3)
		if sel.path == "" {
			sel.path = f.lastA
		}
		f.lastA = sel.path
		if sel.path == "" {
			sel.name = f.st.cf.font
		}
	}
	f.fld.font = sel
	return nil
}

func cmdCF(f *format, _ lexer.Command, ps params) error {
	if ps.has(0) {
		f.st.cf.font = ps.char(0, 'A')
	}
	switch {
	case ps.has(1) && ps.has(2):
		f.st.cf.h, f.st.cf.w = clamp(ps.int(1, 0), 0, f.maxDim()), clamp(ps.int(2, 0), 0, f.maxDim())
	case ps.has(1):
		f.st.cf.h, f.st.cf.w = clamp(ps.int(1, 0), 0, f.maxDim()), 0
	case ps.has(2):
		f.st.cf.h, f.st.cf.w = 0, clamp(ps.int(2, 0), 0, f.maxDim())
	}
	return nil
}

func cmdCW(f *format, _ lexer.Command, ps params) error {
	if ps.has(0) && ps.has(1) {
		f.pr.p.cwAlias[ps.char(0, 0)] = ps.str(1)
	}
	return nil
}

func cmdCI(f *format, _ lexer.Command, ps params) error {
	cs := encoding.Charset{ID: ps.int(0, 0)}
	for i := 1; i+1 < len(ps); i += 2 {
		s, d := ps.int(i, -1), ps.int(i+1, -1)
		if s >= 0 && s < 256 && d >= 0 && d < 256 {
			if cs.Remap == nil {
				cs.Remap = map[byte]byte{}
			}
			cs.Remap[byte(s)] = byte(d)
		}
	}
	f.pr.p.charset = cs
	return nil
}

func cmdCV(f *format, _ lexer.Command, ps params) error {
	f.pr.p.validate = ps.yes(0, false)
	return nil
}

func cmdCM(f *format, _ lexer.Command, ps params) error {
	for i, drive := range []byte{'B', 'E', 'R', 'A'} {
		if a := ps.char(i, 0); a != 0 && a != drive {
			f.pr.st.alias[a] = drive
		}
	}
	return nil
}

func cmdBY(f *format, _ lexer.Command, ps params) error {
	if ps.has(0) {
		f.st.by.module = clamp(ps.int(0, 2), 1, 10)
	}
	if ps.has(1) {
		f.st.by.ratio = min(max(ps.float(1, 3), 2), 3)
	}
	if ps.has(2) {
		f.st.by.height = clamp(ps.int(2, 10), 1, f.maxDim())
	}
	return nil
}

func cmdPA(f *format, _ lexer.Command, ps params) error {
	f.pr.p.pa = advancedLayout{
		defGlyph: ps.int(0, 0) == 1,
		bidi:     ps.int(1, 0) == 1,
		shape:    ps.int(2, 0) == 1,
		features: ps.int(3, 0) == 1,
	}
	return nil
}

func cmdSE(f *format, c lexer.Command, ps params) error {
	o := f.pr.st.get(ps.str(0), "DAT")
	if o == nil || o.raw == nil {
		f.pr.warn(c, "encoding table %s not found", ps.str(0))
		return nil
	}
	f.pr.p.encTable = parseDAT(o.raw)
	return nil
}

// Graphics.

func color(ps params, i int) bool { return ps.char(i, 'B') == 'W' }

func cmdGB(f *format, _ lexer.Command, ps params) error {
	lim := f.maxDim()
	t := clamp(ps.int(2, 1), 1, lim)
	w, h := clamp(max(ps.int(0, t), t), 1, lim), clamp(max(ps.int(1, t), t), 1, lim)
	f.fld.graphics = append(f.fld.graphics, graphic{mask: boxMask(w, h, t, clamp(ps.int(4, 0), 0, 8)), white: color(ps, 3)})
	return nil
}

func cmdGC(f *format, _ lexer.Command, ps params) error {
	d := clamp(ps.int(0, 3), 3, f.maxDim())
	t := clamp(ps.int(1, 1), 1, d)
	f.fld.graphics = append(f.fld.graphics, graphic{mask: ellipseMask(d, d, t), white: color(ps, 2)})
	return nil
}

func cmdGE(f *format, _ lexer.Command, ps params) error {
	lim := f.maxDim()
	t := clamp(ps.int(2, 1), 1, lim)
	w, h := clamp(ps.int(0, t), 3, lim), clamp(ps.int(1, t), 3, lim)
	f.fld.graphics = append(f.fld.graphics, graphic{mask: ellipseMask(w, h, t), white: color(ps, 3)})
	return nil
}

func cmdGD(f *format, _ lexer.Command, ps params) error {
	lim := f.maxDim()
	t := clamp(ps.int(2, 1), 1, lim)
	w, h := clamp(ps.int(0, max(t, 3)), 3, lim), clamp(ps.int(1, max(t, 3)), 3, lim)
	o := ps.char(4, 'R')
	right := o != 'L' && o != '\\'
	f.fld.graphics = append(f.fld.graphics, graphic{mask: diagonalMask(w, h, t, right), white: color(ps, 3)})
	return nil
}

func cmdGF(f *format, c lexer.Command, _ params) error {
	// Split the four header parameters off; the data may contain the
	// delimiter (e.g. ACS ',') or be binary.
	parts := strings.SplitN(string(c.Params), string(c.Delim), 5)
	ps := params(parts)
	kind := ps.char(0, 'A')
	total := ps.int(2, 0)
	row := ps.int(3, 0)
	if row <= 0 || row*8 > f.maxDim() {
		f.pr.warn(c, "invalid bytes per row")
		return nil
	}
	total = min(total, row*f.maxDim())
	data := ps.raw(4)
	var raw []byte
	var err error
	switch kind {
	case 'B':
		raw = []byte(data)
	case 'C':
		f.pr.warn(c, "format C (AR-compressed) graphic data is proprietary; rendered as raw binary")
		raw = []byte(data)
	default:
		raw, err = compress.DecodeGraphic(data, row, total)
		if err != nil {
			f.pr.warn(c, "%v", err)
		}
	}
	if total > 0 && len(raw) > total {
		raw = raw[:total]
	}
	f.fld.graphics = append(f.fld.graphics, graphic{mask: rowsMask(raw, row)})
	return nil
}

func cmdGS(f *format, _ lexer.Command, ps params) error {
	sel := &fontSel{h: clamp(ps.int(1, 0), 0, f.maxDim()), w: clamp(ps.int(2, 0), 0, f.maxDim())}
	sel.rot, sel.rotSet = parseRotation(ps.char(0, 0))
	f.fld.symbol = sel
	return nil
}

// image returns a stored graphic or PNG.
func (f *format) image(c lexer.Command, name string) *bitmap.Bitmap {
	o := f.pr.st.get(name, "GRF")
	if o == nil && !strings.Contains(name, ".") {
		o = f.pr.st.get(name, "PNG")
	}
	if o == nil || o.graphic == nil {
		f.pr.warn(c, "image %s not found", name)
		return nil
	}
	return o.graphic
}

func cmdXG(f *format, c lexer.Command, ps params) error {
	if img := f.image(c, ps.str(0)); img != nil {
		mx, my := clamp(ps.int(1, 1), 1, 10), clamp(ps.int(2, 1), 1, 10)
		f.fld.graphics = append(f.fld.graphics, graphic{mask: img.Scale(mx, my)})
	}
	return nil
}

func cmdIM(f *format, c lexer.Command, ps params) error {
	if img := f.image(c, ps.str(0)); img != nil {
		f.fld.graphics = append(f.fld.graphics, graphic{mask: img})
	}
	return nil
}

func cmdIL(f *format, c lexer.Command, ps params) error {
	if img := f.image(c, ps.str(0)); img != nil {
		f.addOp(op{mask: img, mode: bitmap.Black})
	}
	return nil
}

func cmdIS(f *format, _ lexer.Command, ps params) error {
	f.store = ps.str(0)
	if f.store == "" {
		f.store = "R:UNKNOWN.GRF"
	}
	f.storePrint = ps.yes(1, true)
	return nil
}

func cmdID(f *format, _ lexer.Command, ps params) error {
	f.pr.st.delete(ps.str(0), "GRF")
	return nil
}

func cmdEG(f *format, _ lexer.Command, _ params) error {
	f.pr.st.deleteGraphics()
	return nil
}

func cmdDF(f *format, _ lexer.Command, ps params) error {
	f.fld = newField()
	f.capturing, f.capName, f.captured = true, ps.str(0), nil
	return nil
}

func cmdXF(f *format, c lexer.Command, ps params) error {
	return f.recall(c, ps.str(0))
}

func cmdTO(f *format, c lexer.Command, ps params) error {
	if !ps.has(1) {
		f.pr.warn(c, "missing destination")
		return nil
	}
	if f.pr.st.transfer(ps.str(0), ps.str(1)) == 0 {
		f.pr.warn(c, "no objects match %s", ps.str(0))
	}
	return nil
}

// Label layout.

func cmdLH(f *format, _ lexer.Command, ps params) error {
	f.pr.p.lhX, f.pr.p.lhY = ps.int(0, 0), ps.int(1, 0)
	return nil
}

func cmdLL(f *format, _ lexer.Command, ps params) error {
	if v := ps.int(0, 0); v > 0 {
		f.pr.p.ll = v
	}
	return nil
}

func cmdPW(f *format, _ lexer.Command, ps params) error {
	if v := ps.int(0, 0); v > 0 {
		f.pr.p.pw = v
	}
	return nil
}

func cmdLS(f *format, _ lexer.Command, ps params) error {
	f.pr.p.ls = clamp(ps.int(0, 0), -9999, 9999)
	return nil
}

func cmdLT(f *format, _ lexer.Command, ps params) error {
	if ps.has(0) {
		f.pr.p.lt = clamp(ps.int(0, 0), -240, 240)
	}
	return nil
}

func cmdLR(f *format, _ lexer.Command, ps params) error {
	f.pr.p.reverse = ps.yes(0, false)
	return nil
}

func cmdPO(f *format, _ lexer.Command, ps params) error {
	f.pr.p.invert = ps.char(0, 'N') == 'I'
	return nil
}

func cmdPM(f *format, _ lexer.Command, ps params) error {
	f.pr.p.mirror = ps.yes(0, false)
	return nil
}

func cmdJM(f *format, _ lexer.Command, ps params) error {
	f.pr.p.halfDense = ps.char(0, 'A') == 'B'
	return nil
}

func cmdMC(f *format, _ lexer.Command, ps params) error {
	f.pr.p.keepImage = !ps.yes(0, true)
	return nil
}

// Serialisation and clock.

func cmdSN(f *format, _ lexer.Command, ps params) error {
	start := ps.raw(0)
	if strings.TrimSpace(start) == "" {
		start = "1"
	}
	inc := ps.str(1)
	if inc == "" {
		inc = "1"
	}
	f.fld.serial = &serialSpec{kind: 'N', start: start, inc: inc, zeros: ps.yes(2, false)}
	return nil
}

func cmdSF(f *format, _ lexer.Command, ps params) error {
	f.fld.serial = &serialSpec{kind: 'F', mask: ps.raw(0), inc: ps.raw(1)}
	return nil
}

func cmdSO(f *format, _ lexer.Command, ps params) error {
	which := ps.int(0, 0)
	if which != 2 && which != 3 {
		return nil
	}
	f.pr.p.clockOffs[which] = clockOffset{
		months: ps.int(1, 0), days: ps.int(2, 0), years: ps.int(3, 0),
		hours: ps.int(4, 0), minutes: ps.int(5, 0), seconds: ps.int(6, 0),
	}
	return nil
}

func cmdST(f *format, _ lexer.Command, ps params) error {
	now := f.pr.opt.Now()
	hour := ps.int(3, now.Hour())
	if ps.char(6, 'M') == 'A' && hour < 12 && strings.Contains(strings.ToUpper(ps.raw(6)), "P") {
		hour += 12
	}
	t := time.Date(ps.int(2, now.Year()), time.Month(ps.int(0, int(now.Month()))), ps.int(1, now.Day()),
		hour, ps.int(4, now.Minute()), ps.int(5, now.Second()), 0, now.Location())
	f.pr.p.clockOff = t.Sub(now)
	return nil
}
