package lint

import "regexp"

var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+\.[^@\s]{2,}$`)
	// Banned glyphs are the characters that a PDF text layer or an OCR pass
	// usually turns into noise.
	glyphRe = regexp.MustCompile(`[•▪‣◦·∙–—→⇒★☆✓✔✗✘⚑⚡☎✉➤»«]`)
	emojiRe = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{27BF}\x{FE0F}]`)
	// MetricRe looks for a number, a percentage or a money amount.
	metricRe = regexp.MustCompile(`\d`)
	wordRe   = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'#+./-]*`)
	// French and English articles that must not open a bullet.
	articles = map[string]bool{
		"le": true, "la": true, "les": true, "un": true, "une": true, "des": true,
		"du": true, "de": true, "the": true, "a": true, "an": true,
	}
)
