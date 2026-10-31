package engine

import (
	"path"
	"sort"
	"strings"

	"github.com/datamagik/zpl-to-image/internal/bitmap"
	"github.com/datamagik/zpl-to-image/internal/font"
	"github.com/datamagik/zpl-to-image/internal/lexer"
)

// object is a file on one of the printer's virtual drives.
type object struct {
	graphic  *bitmap.Bitmap
	format   []lexer.Command
	scalable *font.Scalable
	bitmapFt *font.Downloaded
	raw      []byte
}

// storage models the printer drives (R:, E:, B:, A:, Z:). Names are
// normalised to upper case "D:NAME.EXT".
type storage struct {
	objects map[string]*object
	// alias maps a drive letter to another (^CM).
	alias map[byte]byte
}

func newStorage() *storage {
	return &storage{objects: map[string]*object{}, alias: map[byte]byte{}}
}

// normalize turns "d:o.x" with defaults into "D:O.X". The default drive is
// R: and defExt is applied when the name has no extension.
func (s *storage) normalize(name, defExt string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	drive := "R"
	if i := strings.IndexByte(name, ':'); i >= 0 {
		if i > 0 {
			drive = name[:i]
		}
		name = name[i+1:]
	}
	if len(drive) == 1 {
		if a, ok := s.alias[drive[0]]; ok {
			drive = string(a)
		}
	}
	if name == "" {
		name = "UNKNOWN"
	}
	if !strings.Contains(name, ".") && defExt != "" {
		name += "." + strings.ToUpper(defExt)
	}
	return drive + ":" + name
}

func (s *storage) put(name, defExt string, o *object) string {
	key := s.normalize(name, defExt)
	s.objects[key] = o
	return key
}

// get looks name up on its drive and then, like the printer's search of
// all drives, by file name on any drive.
func (s *storage) get(name, defExt string) *object {
	key := s.normalize(name, defExt)
	if o, ok := s.objects[key]; ok {
		return o
	}
	file := key[strings.IndexByte(key, ':')+1:]
	for _, k := range s.sortedKeys() {
		if k[strings.IndexByte(k, ':')+1:] == file {
			return s.objects[k]
		}
	}
	return nil
}

func (s *storage) sortedKeys() []string {
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// match returns the stored keys matching a pattern with * and ?
// wildcards; defExt applies when the pattern has no extension.
func (s *storage) match(pattern, defExt string) []string {
	pat := s.normalize(pattern, defExt)
	var out []string
	for _, k := range s.sortedKeys() {
		if ok, _ := path.Match(pat, k); ok {
			out = append(out, k)
		}
	}
	return out
}

// delete removes objects matching a pattern with * and ? wildcards.
func (s *storage) delete(pattern, defExt string) {
	for _, k := range s.match(pattern, defExt) {
		delete(s.objects, k)
	}
}

// transfer copies objects matching src (^TO source, whose name and
// extension default to *) to dst. A destination without a name or
// extension keeps the source's.
func (s *storage) transfer(src, dst string) int {
	src = strings.TrimSpace(src)
	if src == "" || strings.HasSuffix(src, ":") {
		src += "*"
	}
	dDrive, dName := "R", strings.ToUpper(strings.TrimSpace(dst))
	if i := strings.IndexByte(dName, ':'); i >= 0 {
		if i > 0 {
			dDrive = dName[:i]
		}
		dName = dName[i+1:]
	}
	n := 0
	for _, k := range s.match(src, "*") {
		file := k[strings.IndexByte(k, ':')+1:]
		base, ext, _ := strings.Cut(file, ".")
		name, dExt, hasExt := strings.Cut(dName, ".")
		if name == "" || name == "*" {
			name = base
		}
		if !hasExt || dExt == "" || dExt == "*" {
			dExt = ext
		}
		cp := *s.objects[k]
		s.objects[dDrive+":"+name+"."+dExt] = &cp
		n++
	}
	return n
}

// deleteGraphics removes all .GRF objects (~EG / ^EG).
func (s *storage) deleteGraphics() {
	for _, k := range s.sortedKeys() {
		if strings.HasSuffix(k, ".GRF") {
			delete(s.objects, k)
		}
	}
}
