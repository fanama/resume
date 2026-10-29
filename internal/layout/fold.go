package layout

import "strings"

// Fold removes diacritics from a string, keeping non-latin scripts intact.
func Fold(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if folded, ok := foldRune(r); ok {
			b.WriteRune(folded)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// foldTable maps a latin letter carrying a diacritic to its ASCII base letter.
// Letters without an ASCII equivalent (the German sharp s) are absent on
// purpose, and so is every non-latin script: they are copied unchanged.
var foldTable = map[rune]rune{
	'À': 'A', 'Á': 'A', 'Â': 'A', 'Ã': 'A', 'Ä': 'A', 'Å': 'A', 'Ā': 'A', 'Ă': 'A', 'Ą': 'A',
	'Ç': 'C', 'Ć': 'C', 'Č': 'C',
	'Ď': 'D', 'Đ': 'D',
	'È': 'E', 'É': 'E', 'Ê': 'E', 'Ë': 'E', 'Ē': 'E', 'Ĕ': 'E', 'Ė': 'E', 'Ę': 'E', 'Ě': 'E',
	'Ì': 'I', 'Í': 'I', 'Î': 'I', 'Ï': 'I', 'Ĩ': 'I', 'Ī': 'I', 'Ĭ': 'I', 'Į': 'I', 'İ': 'I',
	'Ł': 'L',
	'Ñ': 'N', 'Ń': 'N', 'Ņ': 'N',
	'Ò': 'O', 'Ó': 'O', 'Ô': 'O', 'Õ': 'O', 'Ö': 'O', 'Ø': 'O', 'Ō': 'O', 'Ŏ': 'O', 'Ő': 'O',
	'Œ': 'O', 'Æ': 'A',
	'Ŕ': 'R', 'Ř': 'R',
	'Ś': 'S', 'Ş': 'S', 'Š': 'S',
	'Ţ': 'T', 'Ť': 'T', 'Ŧ': 'T', 'Þ': 'T',
	'Ù': 'U', 'Ú': 'U', 'Û': 'U', 'Ü': 'U', 'Ũ': 'U', 'Ū': 'U', 'Ŭ': 'U', 'Ů': 'U', 'Ű': 'U', 'Ų': 'U',
	'Ŵ': 'W',
	'Ý': 'Y', 'Ŷ': 'Y', 'Ÿ': 'Y',
	'Ź': 'Z', 'Ż': 'Z', 'Ž': 'Z',

	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
	'ç': 'c', 'ć': 'c', 'č': 'c',
	'ď': 'd', 'đ': 'd', 'ð': 'd',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ĕ': 'e', 'ė': 'e', 'ę': 'e', 'ě': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ĩ': 'i', 'ī': 'i', 'ĭ': 'i', 'į': 'i', 'ı': 'i',
	'ł': 'l',
	'ñ': 'n', 'ń': 'n', 'ņ': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'ō': 'o', 'ŏ': 'o', 'ő': 'o',
	'œ': 'o', 'æ': 'a',
	'ŕ': 'r', 'ř': 'r',
	'ś': 's', 'ş': 's', 'š': 's',
	'ţ': 't', 'ť': 't', 'ŧ': 't',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ũ': 'u', 'ū': 'u', 'ŭ': 'u', 'ů': 'u', 'ű': 'u', 'ų': 'u',
	'ŵ': 'w',
	'ý': 'y', 'ŷ': 'y', 'ÿ': 'y',
	'ź': 'z', 'ż': 'z', 'ž': 'z',
}

// foldRune returns the ASCII base of a latin letter, if it has one.
func foldRune(r rune) (rune, bool) {
	folded, ok := foldTable[r]
	return folded, ok
}
