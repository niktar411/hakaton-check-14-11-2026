package importer

import (
	"regexp"
	"strings"
	"unicode"
)

// Transliterate lowers the input and converts Cyrillic letters to Latin so the
// same software is matched regardless of the export language.
func Transliterate(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if replacement, ok := translit[r]; ok {
			b.WriteString(replacement)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'і': "i", 'ї': "yi", 'є': "ye", 'ґ': "g", 'ў': "u",
}

// NormalizeName produces a comparison key for a product name: case, script,
// punctuation, architecture suffixes and trademark symbols do not matter.
func NormalizeName(s string) string {
	normalized := archToken.ReplaceAllString(normalizeText(s), " ")
	return strings.Join(strings.Fields(normalized), " ")
}

// NormalizeVendor removes punctuation, legal suffixes and the leading article.
func NormalizeVendor(s string) string {
	words := strings.Fields(normalizeText(s))
	words = trimLegalPrefixes(words)
	words = trimLegalSuffixes(words)
	return strings.Join(words, " ")
}

// NormalizeVersion makes "v1.02.0.0", "1_2" and "1.2.0" comparable. Build
// metadata and a parenthesised architecture are ignored.
func NormalizeVersion(s string) string {
	normalized := Transliterate(strings.TrimSpace(s))
	if i := strings.IndexAny(normalized, "(+"); i >= 0 {
		normalized = normalized[:i]
	}
	normalized = strings.TrimSpace(strings.TrimPrefix(normalized, "version"))
	switch {
	case strings.HasPrefix(normalized, "v."):
		normalized = normalized[2:]
	case len(normalized) > 1 && normalized[0] == 'v' && normalized[1] >= '0' && normalized[1] <= '9':
		normalized = normalized[1:]
	}

	var b strings.Builder
	b.Grow(len(normalized))
	for _, r := range normalized {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-' || r == ',' || r == ' ':
			b.WriteByte('.')
		}
	}
	parts := strings.Split(strings.Trim(b.String(), "."), ".")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if isDigits(part) {
			part = strings.TrimLeft(part, "0")
			if part == "" {
				part = "0"
			}
		}
		cleaned = append(cleaned, part)
	}
	for len(cleaned) > 1 && cleaned[len(cleaned)-1] == "0" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return strings.Join(cleaned, ".")
}

// NormalizeArchitecture maps the spellings found in inventory exports to four
// stable values and leaves unknown values as compact lower-case text.
func NormalizeArchitecture(s string) string {
	normalized := strings.ReplaceAll(normalizeText(s), " ", "")
	switch normalized {
	case "amd64", "x64", "x8664", "64bit", "64":
		return "amd64"
	case "x86", "i386", "i486", "i586", "i686", "386", "x32", "32bit", "32":
		return "386"
	case "arm64", "aarch64":
		return "arm64"
	case "arm", "arm32", "armv7":
		return "arm"
	case "any", "noarch", "neutral", "universal", "all":
		return "any"
	}
	switch {
	case strings.Contains(normalized, "x8664") || strings.HasSuffix(normalized, "64bit"):
		return "amd64"
	case strings.Contains(normalized, "arm64") || strings.Contains(normalized, "aarch64"):
		return "arm64"
	case strings.Contains(normalized, "x86") || strings.HasSuffix(normalized, "32bit"):
		return "386"
	default:
		return normalized
	}
}

// NormalizeUpdate returns a stable update identifier. A KB number is the most
// useful value, so it wins over the rest of the text.
func NormalizeUpdate(s string) string {
	normalized := normalizeText(s)
	if kb := kbToken.FindString(normalized); kb != "" {
		return strings.ToUpper(kb)
	}
	return normalized
}

// IsUpdate reports whether a row describes a patch, KB or update package
// rather than an installed product. These rows are not trackable assets.
func IsUpdate(row Row) bool {
	update := normalizeText(row.Update)
	switch update {
	case "", "n a", "none", "not applicable", "no":
	default:
		if kbToken.MatchString(update) || containsAny(update, "update", "hotfix", "patch") {
			return true
		}
	}
	text := normalizeText(row.Name + " " + row.Publisher)
	if kbToken.MatchString(text) {
		return true
	}
	for _, pattern := range updatePatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// DedupeKey collapses rows with the same device, product and version. The
// separator cannot appear in normalized fields, so keys cannot collide.
func DedupeKey(row Row) string {
	return strings.Join([]string{
		NormalizeDevice(row.Device),
		NormalizeName(row.Name),
		NormalizeVersion(row.Version),
	}, "\x1f")
}

// NormalizeDevice compares short and FQDN host names by dropping the domain
// and all separators.
func NormalizeDevice(s string) string {
	device := strings.TrimSpace(s)
	if i := strings.IndexByte(device, '.'); i > 0 {
		device = device[:i]
	}
	return strings.ReplaceAll(normalizeText(device), " ", "")
}

var (
	archToken = regexp.MustCompile(`\b(x86\s*64|amd64|x64|aarch64|arm64|arm32|x86|i[3-6]86|x32|32\s*bit|64\s*bit|noarch|anycpu)\b`)
	kbToken   = regexp.MustCompile(`\bkb\s*\d{4,}\b`)

	updatePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bupdate for\b`),
		regexp.MustCompile(`\bsecurity update\b`),
		regexp.MustCompile(`\bcumulative update\b`),
		regexp.MustCompile(`\bdefinition update\b`),
		regexp.MustCompile(`\bfeature update\b`),
		regexp.MustCompile(`\bservicing stack\b`),
		regexp.MustCompile(`\bupdate rollup\b`),
		regexp.MustCompile(`\bhotfix\b`),
		regexp.MustCompile(`\bservice pack\b`),
		regexp.MustCompile(`\bpatch for\b`),
	}
)

func normalizeText(s string) string {
	s = strings.NewReplacer("™", " ", "®", " ", "©", " ").Replace(strings.TrimSpace(s))
	s = Transliterate(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '&':
			b.WriteString(" and ")
		case r == '+':
			b.WriteString(" plus ")
		case r == '#':
			b.WriteString(" sharp ")
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func trimLegalPrefixes(words []string) []string {
	legal := map[string]struct{}{
		"the": {}, "ooo": {}, "ao": {}, "pao": {}, "zao": {}, "oao": {}, "ip": {},
	}
	for len(words) > 0 {
		if _, ok := legal[words[0]]; !ok {
			break
		}
		words = words[1:]
	}
	return words
}

func trimLegalSuffixes(words []string) []string {
	legal := map[string]struct{}{
		"inc": {}, "incorporated": {}, "llc": {}, "ltd": {}, "limited": {},
		"corp": {}, "corporation": {}, "co": {}, "company": {}, "gmbh": {},
		"ag": {}, "sa": {}, "sas": {}, "bv": {}, "nv": {}, "oy": {}, "ab": {},
		"plc": {}, "pty": {},
	}
	for len(words) > 0 {
		if _, ok := legal[words[len(words)-1]]; !ok {
			break
		}
		words = words[:len(words)-1]
	}
	return words
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
