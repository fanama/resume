package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	fpdf "github.com/go-pdf/fpdf"
)

// fontDirs are the usual places a TrueType file lives, macOS first.
var fontDirs = []string{
	"/System/Library/Fonts/Supplemental",
	"/Library/Fonts",
	"/Library/Fonts/Microsoft",
	"/usr/share/fonts/truetype/dejavu",
	"/usr/share/fonts/truetype/liberation",
	"/usr/share/fonts/TTF",
	"/usr/share/fonts",
	"/usr/local/share/fonts",
}

func addTTF(pdf *fpdf.Fpdf, family, style, path string) error {
	data, err := readFont(path)
	if err != nil {
		return fmt.Errorf("pdf: read font %s: %w", path, err)
	}
	pdf.AddUTF8FontFromBytes(family, style, data)
	return nil
}

// The font files do not change between two renders of the same process: the
// resolution above is a hundred stat calls and the read is more than a
// megabyte of TrueType data that fpdf then parses again. Both answers are
// memoised for the life of the process, so only the first document of a
// session pays for them. A font file edited under a running server is a
// deployment, not a request.
var (
	fontPathMu sync.Mutex
	fontPaths  = map[fontKey][2]string{}
	fontDataMu sync.Mutex
	fontData   = map[string][]byte{}
)

// fontKey carries every option that decides which files are used.
type fontKey struct {
	family string
	dir    string
	file   string
	bold   string
}

// readFont returns the contents of a TrueType file, read at most once. The
// bytes handed back are a copy: the renderer must not be able to write into
// the cache another renderer is still reading from.
func readFont(path string) ([]byte, error) {
	fontDataMu.Lock()
	cached, ok := fontData[path]
	fontDataMu.Unlock()
	if ok {
		return append([]byte(nil), cached...), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fontDataMu.Lock()
	fontData[path] = data
	fontDataMu.Unlock()
	return data, nil
}

// resolveFontFiles looks for a regular and a bold TrueType file. An explicit
// path always wins. The lookup stats every spelling of the family in every
// font directory, so its answer is cached under the options that produced it.
func resolveFontFiles(opts Options) (regular, bold string) {
	key := fontKey{family: opts.FontFamily, dir: opts.FontDir, file: opts.FontFile, bold: opts.BoldFontFile}
	fontPathMu.Lock()
	cached, ok := fontPaths[key]
	fontPathMu.Unlock()
	if ok {
		return cached[0], cached[1]
	}
	regular, bold = resolveFontFilesUncached(opts)
	fontPathMu.Lock()
	fontPaths[key] = [2]string{regular, bold}
	fontPathMu.Unlock()
	return regular, bold
}

func resolveFontFilesUncached(opts Options) (regular, bold string) {
	if opts.FontFile != "" {
		regular = opts.FontFile
	}
	if opts.BoldFontFile != "" {
		bold = opts.BoldFontFile
	}
	dirs := append([]string{}, fontDirs...)
	if opts.FontDir != "" {
		dirs = append([]string{opts.FontDir}, dirs...)
	}
	if regular == "" {
		regular = lookup(dirs, opts.FontFamily, false)
	}
	if bold == "" {
		bold = lookup(dirs, opts.FontFamily, true)
	}
	return regular, bold
}

// lookup walks the font directories looking for a family, in the "Arial.ttf",
// "Arial Bold.ttf", "arial-bold.ttf" spellings the platforms use.
func lookup(dirs []string, family string, bold bool) string {
	bases := []string{family}
	if tight := strings.ReplaceAll(family, " ", ""); tight != family {
		bases = append(bases, tight)
	}
	styles := []string{""}
	if bold {
		styles = []string{" Bold", "-Bold", "Bold", " bold", "-bold", "bd"}
	}
	for _, dir := range dirs {
		for _, base := range bases {
			for _, style := range styles {
				for _, name := range []string{base + style + ".ttf", strings.ToLower(base + style + ".ttf")} {
					path := filepath.Join(dir, name)
					if info, err := os.Stat(path); err == nil && !info.IsDir() {
						return path
					}
				}
			}
		}
	}
	return ""
}

// CP1252 converts a UTF-8 string to the single byte encoding of the standard
// PDF fonts. Characters outside the encoding become a question mark, which is
// the honest outcome: there is no glyph to draw. A tabulation becomes a space
// because a tab is a column for a text extractor; a line feed is kept, fpdf
// needs it to break a paragraph.
func CP1252(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x80, r >= 0xA0 && r <= 0xFF:
			// ASCII, the line feed fpdf needs, or Latin-1, which cp1252
			// shares.
			b.WriteByte(byte(r))
		default:
			if c, ok := cp1252High[r]; ok {
				b.WriteByte(c)
			} else {
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// cp1252High is the 0x80 to 0x9F range of cp1252, the only part that differs
// from Latin-1.
var cp1252High = map[rune]byte{
	0x20AC: 0x80, 0x201A: 0x82, 0x0192: 0x83, 0x201E: 0x84, 0x2026: 0x85,
	0x2020: 0x86, 0x2021: 0x87, 0x02C6: 0x88, 0x2030: 0x89, 0x0160: 0x8A,
	0x2039: 0x8B, 0x0152: 0x8C, 0x017D: 0x8E, 0x2018: 0x91, 0x2019: 0x92,
	0x201C: 0x93, 0x201D: 0x94, 0x2022: 0x95, 0x2013: 0x96, 0x2014: 0x97,
	0x02DC: 0x98, 0x2122: 0x99, 0x0161: 0x9A, 0x203A: 0x9B, 0x0153: 0x9C,
	0x017E: 0x9E, 0x0178: 0x9F,
}
