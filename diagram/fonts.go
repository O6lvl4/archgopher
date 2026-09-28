package diagram

import (
	"os"
	"runtime"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// fontSet holds the faces the raster draws with: the Go fonts, embedded, and
// a CJK font from the system when one is found, for labels the Go fonts
// cannot show. ARCHGOPHER_FONT names a font file to use for those instead.
type fontSet struct {
	regular, bold, mono, cjk *sfnt.Font
	mu                       sync.Mutex
	faces                    map[faceKey]font.Face
}

type faceKey struct {
	f    *sfnt.Font
	size float64
}

var (
	fontsOnce sync.Once
	fonts     *fontSet
)

// cjkCandidates are the system fonts tried, in order, for CJK text.
var cjkCandidates = map[string][]string{
	"darwin": {
		"/System/Library/Fonts/ヒラギノ角ゴシック W4.ttc",
		"/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/Library/Fonts/Arial Unicode.ttf",
	},
	"linux": {
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/ipafont-gothic/ipag.ttf",
	},
	"windows": {
		`C:\Windows\Fonts\YuGothM.ttc`,
		`C:\Windows\Fonts\meiryo.ttc`,
		`C:\Windows\Fonts\msgothic.ttc`,
	},
}

// loadFonts parses the embedded fonts once and looks for a CJK font.
func loadFonts() *fontSet {
	fontsOnce.Do(func() {
		fonts = &fontSet{faces: map[faceKey]font.Face{}}
		fonts.regular, _ = sfnt.Parse(goregular.TTF)
		fonts.bold, _ = sfnt.Parse(gobold.TTF)
		fonts.mono, _ = sfnt.Parse(gomono.TTF)
		candidates := cjkCandidates[runtime.GOOS]
		if p := os.Getenv("ARCHGOPHER_FONT"); p != "" {
			candidates = append([]string{p}, candidates...)
		}
		for _, p := range candidates {
			if f := readFont(p); f != nil {
				fonts.cjk = f
				break
			}
		}
	})
	return fonts
}

// readFont parses a .ttf, .otf or .ttc file; nil when it cannot.
func readFont(path string) *sfnt.Font {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if coll, err := sfnt.ParseCollection(data); err == nil && coll.NumFonts() > 0 {
		if f, err := coll.Font(0); err == nil {
			return f
		}
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil
	}
	return f
}

// face is the face for a text op at a pixel size.
func (fs *fontSet) face(o op, size float64) font.Face {
	f := fs.regular
	switch {
	case fs.cjk != nil && needsCJK(o.text):
		f = fs.cjk
	case o.mono:
		f = fs.mono
	case o.bold:
		f = fs.bold
	}
	if f == nil {
		return nil
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	key := faceKey{f, size}
	if face, ok := fs.faces[key]; ok {
		return face
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	fs.faces[key] = face
	return face
}

// needsCJK reports whether a string has characters outside the Go fonts'
// coverage: kana, CJK ideographs, fullwidth forms.
func needsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x2E80 && r <= 0x9FFF || r >= 0xF900 && r <= 0xFAFF || r >= 0xFF00 && r <= 0xFFEF {
			return true
		}
	}
	return false
}
