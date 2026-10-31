// Package lexer splits a ZPL II byte stream into commands.
//
// ZPL commands start with the format prefix (default '^') or the control
// prefix (default '~') followed by a command name; parameters run until the
// next prefix character. The lexer tracks prefix and delimiter changes made
// by ^CC/~CC, ^CT/~CT and ^CD/~CD, strips CR/LF from text parameters (as the
// printer does), and reads binary payloads of ^GF and ~DY by byte count.
package lexer

import (
	"bytes"
	"strconv"
	"strings"
)

// Command is one lexed ZPL command.
type Command struct {
	// Control is true for control-prefix ('~') commands.
	Control bool
	// Name is the upper-cased command name without prefix, e.g. "FO", "GF",
	// "A" (font select; the font name is the first byte of Params).
	Name string
	// Params is the raw parameter text with CR/LF removed, except for binary
	// payloads which are passed through untouched.
	Params []byte
	// Delim is the parameter delimiter in effect for this command.
	Delim byte
	// Offset is the byte offset of the prefix in the input.
	Offset int
}

// Key returns the canonical command key, e.g. "^FO" or "~DG".
func (c Command) Key() string {
	if c.Control {
		return "~" + c.Name
	}
	return "^" + c.Name
}

// Split splits Params on the delimiter. An empty parameter list yields nil.
func (c Command) Split() []string {
	if len(c.Params) == 0 {
		return nil
	}
	return strings.Split(string(c.Params), string(c.Delim))
}

// Lexer produces Commands from a byte slice.
type Lexer struct {
	src     []byte
	pos     int
	format  byte // '^' by default
	control byte // '~' by default
	delim   byte // ',' by default
}

// New returns a lexer over src.
func New(src []byte) *Lexer {
	return &Lexer{src: src, format: '^', control: '~', delim: ','}
}

func (l *Lexer) isPrefix(b byte) bool { return b == l.format || b == l.control }

// Next returns the next command. ok is false at end of input. Bytes outside
// commands (before the first prefix) are skipped.
func (l *Lexer) Next() (cmd Command, ok bool) {
	for l.pos < len(l.src) && !l.isPrefix(l.src[l.pos]) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		return Command{}, false
	}
	start := l.pos
	control := l.src[l.pos] == l.control && l.control != l.format
	l.pos++
	name := l.readName()
	cmd = Command{Control: control, Name: name, Delim: l.delim, Offset: start}

	switch name {
	case "CC", "CT", "CD":
		// The new character follows the command directly.
		ch := l.nextRaw()
		cmd.Params = []byte{}
		if ch >= 0 {
			cmd.Params = []byte{byte(ch)}
			l.apply(name, byte(ch))
		}
		l.skipToPrefix()
		return cmd, true
	case "GF":
		if !control {
			cmd.Params = l.readGF()
			return cmd, true
		}
	case "DY":
		if control {
			cmd.Params = l.readDY()
			return cmd, true
		}
	}
	cmd.Params = l.readText()
	return cmd, true
}

// readName reads a command name: two characters, except ^A whose second
// character is the font name and is left in the parameters. Names skip any
// CR/LF the host inserted.
func (l *Lexer) readName() string {
	var b [2]byte
	n := 0
	for n < 2 && l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\r' || c == '\n' {
			l.pos++
			continue
		}
		if l.isPrefix(c) {
			break
		}
		if n == 0 && (c == 'A' || c == 'a') {
			// ^A<font> — but ^A@ too. Only the format-prefix ^A is font
			// select; there is no other command starting with A.
			l.pos++
			return "A"
		}
		b[n] = upper(c)
		n++
		l.pos++
	}
	return string(b[:n])
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// nextRaw returns the next non-CR/LF byte or -1.
func (l *Lexer) nextRaw() int {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		l.pos++
		if c != '\r' && c != '\n' {
			return int(c)
		}
	}
	return -1
}

func (l *Lexer) apply(name string, ch byte) {
	switch name {
	case "CC":
		l.format = ch
	case "CT":
		l.control = ch
	case "CD":
		l.delim = ch
	}
}

func (l *Lexer) skipToPrefix() {
	for l.pos < len(l.src) && !l.isPrefix(l.src[l.pos]) {
		l.pos++
	}
}

// readText reads parameter text until the next prefix, dropping CR/LF.
func (l *Lexer) readText() []byte {
	var out []byte
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if l.isPrefix(c) {
			break
		}
		if c != '\r' && c != '\n' {
			out = append(out, c)
		}
		l.pos++
	}
	if out == nil {
		out = []byte{}
	}
	return out
}

// readFields reads n delimiter-terminated text fields (CR/LF dropped) and
// returns them with the delimiters. It stops early at a prefix.
func (l *Lexer) readFields(n int) (head []byte, fields []string, complete bool) {
	field := []byte{}
	for l.pos < len(l.src) && len(fields) < n {
		c := l.src[l.pos]
		if l.isPrefix(c) {
			return head, append(fields, string(field)), false
		}
		l.pos++
		if c == '\r' || c == '\n' {
			continue
		}
		head = append(head, c)
		if c == l.delim {
			fields = append(fields, string(field))
			field = field[:0]
			continue
		}
		field = append(field, c)
	}
	return head, fields, len(fields) == n
}

// readGF handles ^GFa,b,c,d,data. Formats B and C carry b raw bytes.
func (l *Lexer) readGF() []byte {
	head, f, ok := l.readFields(4)
	if !ok {
		return head
	}
	format := strings.ToUpper(strings.TrimSpace(f[0]))
	if format == "B" || format == "C" {
		n, err := strconv.Atoi(strings.TrimSpace(f[1]))
		if err == nil && n >= 0 {
			return append(head, l.readBinary(n)...)
		}
	}
	return append(head, l.readText()...)
}

// readDY handles ~DYd:f,b,x,t,w,data. Formats B and C carry t raw bytes.
func (l *Lexer) readDY() []byte {
	head, f, ok := l.readFields(5)
	if !ok {
		return head
	}
	format := strings.ToUpper(strings.TrimSpace(f[1]))
	if format == "B" || format == "C" {
		n, err := strconv.Atoi(strings.TrimSpace(f[3]))
		if err == nil && n >= 0 {
			return append(head, l.readBinary(n)...)
		}
	}
	return append(head, l.readText()...)
}

func (l *Lexer) readBinary(n int) []byte {
	end := min(l.pos+n, len(l.src))
	b := bytes.Clone(l.src[l.pos:end])
	l.pos = end
	return b
}

// All lexes the whole input.
func All(src []byte) []Command {
	l := New(src)
	var out []Command
	for {
		c, ok := l.Next()
		if !ok {
			return out
		}
		out = append(out, c)
	}
}
