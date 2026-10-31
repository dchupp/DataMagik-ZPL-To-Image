package engine

import (
	"strings"
	"testing"
)

// zebraIndex is every command in the Zebra ZPL II Programming Guide command
// index, plus the wireless/RFID chapter commands.
const zebraIndex = `^A ^A@ ^B0 ^B1 ^B2 ^B3 ^B4 ^B5 ^B7 ^B8 ^B9 ^BA ^BB ^BC ^BD ^BE ^BF ^BI ^BJ ^BK
^BL ^BM ^BO ^BP ^BQ ^BR ^BS ^BT ^BU ^BX ^BY ^BZ ^CC ~CC ^CD ~CD ^CF ^CI ^CM ^CN ^CO ^CP ^CT ~CT
^CV ^CW ~DB ~DE ^DF ~DG ~DN ~DS ~DT ~DU ~DY ~EG ^FB ^FC ^FD ^FE ^FH ^FL ^FM ^FN ^FO ^FP ^FR
^FS ^FT ^FV ^FW ^FX ^GB ^GC ^GD ^GE ^GF ^GS ~HB ~HD ^HF ^HG ^HH ~HI ~HM ~HQ ~HS ^HT ~HU ^HV
^HW ^HY ^HZ ^ID ^IL ^IM ^IS ~JA ^JB ~JB ~JC ~JD ~JE ~JF ~JG ^JH ^JI ~JI ^JJ ~JL ^JM ~JN ~JO
~JP ~JQ ~JR ^JS ~JS ^JT ^JU ^JW ~JX ^JZ ~KB ^KD ^KL ^KN ^KP ^KV ^LF ^LH ^LL ^LR ^LS ^LT ^MA
^MC ^MD ^MF ^MI ^ML ^MM ^MN ^MP ^MT ^MU ^MW ^PA ^PF ^PH ~PH ~PL ^PM ~PM ^PN ^PO ^PP ~PP ^PQ
~PR ^PR ~PS ^PW ~RO ^SC ~SD ^SE ^SF ^SI ^SL ^SN ^SO ^SP ^SQ ^SR ^SS ^ST ^SX ^SZ ~TA ^TB ^TO
~WC ^WD ~WQ ^XA ^XB ^XF ^XG ^XS ^XZ ^ZZ
^HL ~HL ^HR ^RB ^RF ^RL ^RS ^RU ^RW ^KC ^NB ^NC ~NC ^ND ^NI ^NN ^NP ^NR ^NS ^NT ~NT ^NW
^WA ^WE ^WL ~WL ^WP ^WR ~WR ^WS ^WX`

func TestEveryCommandHandled(t *testing.T) {
	for _, c := range strings.Fields(zebraIndex) {
		key := c
		if c == "^A@" {
			key = "^A"
		}
		if c == "^NR" {
			key = "~NR"
		}
		if _, ok := handlers[key]; !ok {
			t.Errorf("no handler for %s", c)
		}
	}
}
