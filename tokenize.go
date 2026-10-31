package zpl

import (
	"io"
	"strings"

	"github.com/dchupp/DataMagik-ZPL-To-Image/internal/lexer"
)

// Command is one ZPL command as the renderer reads it, returned by Tokenize.
type Command struct {
	// Prefix is '^' for format commands and '~' for control commands. It is
	// always the canonical character, even when the input changed the
	// prefixes with ^CC/~CC or ^CT/~CT.
	Prefix byte
	// Name is the upper-cased command name without its prefix, e.g. "FO",
	// "GF" or "XA". Font selection (^A0N, ^AD, ^A@) has Name "A": the font
	// name is part of the first parameter (see Params).
	Name string
	// Params are the parameters split on the delimiter in effect (',' unless
	// changed with ^CD/~CD), with CR/LF removed and spaces kept. A command
	// without parameters has none.
	//
	//   - ^A: the first parameter is the font name followed by the
	//     orientation: ^A0N,30,20 gives ["0N", "30", "20"] and ^A@N,30,30,E:X.TTF
	//     gives ["@N", "30", "30", "E:X.TTF"]. The font name is Params[0][0].
	//   - ^CC, ^CT, ^CD (and ~CC, ~CT, ~CD): the one new character.
	//   - ^FD, ^FV and ^FX: none; the text is in Data.
	//   - ^GF, ~DG, ~DY, ~DB, ~DU, ~DT, ~DS, ~DE: the header parameters
	//     only; the payload is in Data.
	Params []string
	// Data is the field data of ^FD and ^FV, the comment of ^FX, the start
	// value of ^SN (also Params[0]), or the payload of a graphic, font or
	// object download (^GF, ~DG, ~DY, ~DB, ~DU, ~DT, ~DS, ~DE).
	//
	// Field data runs to the next format or control prefix, normally the
	// ^FS, exactly as the renderer and the printer read it; delimiters in it
	// are data. CR/LF are removed. ^FH hexadecimal escapes (e.g. "_1D") are
	// left as written: they are decoded when the field is rendered. Binary
	// payloads (^GF and ~DY formats B and C) are returned byte for byte.
	Data string
	// Offset and End are byte offsets into the source: source[Offset:End]
	// is the command exactly as written, from its prefix up to the next
	// command, including any CR/LF, spaces and binary data. Commands are in
	// order and never overlap. The bytes between one command's End and the
	// next one's Offset (text before the first command, or stray bytes after
	// a counted binary payload) are not part of any command.
	Offset, End int
}

// payloadHeaders is the number of header parameters before the payload of
// each download command.
var payloadHeaders = map[string]int{
	"^GF": 4, "~DG": 3, "~DY": 5, "~DB": 8, "~DU": 2, "~DT": 2, "~DS": 2, "~DE": 2,
}

// Tokenize splits ZPL into commands without interpreting them. It uses the
// renderer's own lexer, so it sees exactly the commands Render does,
// tracking ^CC/^CT/^CD prefix and delimiter changes. The only error is one
// from reading r; malformed input still tokenizes.
func Tokenize(r io.Reader) ([]Command, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	lexed := lexer.All(src)
	out := make([]Command, 0, len(lexed))
	for _, c := range lexed {
		out = append(out, publicCommand(c))
	}
	return out, nil
}

func publicCommand(c lexer.Command) Command {
	cmd := Command{Prefix: '^', Name: c.Name, Offset: c.Offset, End: c.End}
	if c.Control {
		cmd.Prefix = '~'
	}
	key := c.Key()
	switch {
	case key == "^FD" || key == "^FV" || key == "^FX":
		cmd.Data = string(c.Params)
	case payloadHeaders[key] > 0:
		n := payloadHeaders[key]
		parts := strings.SplitN(string(c.Params), string(c.Delim), n+1)
		if len(parts) > n {
			cmd.Params, cmd.Data = parts[:n], parts[n]
		} else if len(c.Params) > 0 {
			cmd.Params = parts
		}
	case c.Name == "CC" || c.Name == "CT" || c.Name == "CD":
		if len(c.Params) > 0 {
			cmd.Params = []string{string(c.Params)}
		}
	default:
		cmd.Params = c.Split()
		if key == "^SN" && len(cmd.Params) > 0 {
			cmd.Data = cmd.Params[0]
		}
	}
	return cmd
}
