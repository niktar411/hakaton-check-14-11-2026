package matching

import (
	"regexp"
	"strings"
)

const matchThreshold = 0.7

var (
	punctuation = strings.NewReplacer(
		"_", " ", ".", " ", "-", " ", "(", " ", ")", " ", "[", " ", "]", " ",
		"/", " ", "\\", " ", "+", " ", ",", " ", ":", " ", ";", " ", "\"", " ",
		"'", " ", "«", " ", "»", " ", "|", " ", "—", " ", "–", " ",
	)
	versionPattern = regexp.MustCompile(`^(v?\d+([._]\d+)*|kb\d+|19\d\d|20\d\d)$`)
)

var noiseTokens = map[string]bool{
	"exe": true, "msi": true, "dll": true, "app": true, "application": true,
	"setup": true, "install": true, "installer": true, "update": true, "updater": true,
	"x64": true, "x86": true, "64": true, "32": true, "64bit": true, "32bit": true,
	"v1": true, "v2": true, "v3": true, "v4": true, "v5": true,
	"ru": true, "en": true, "pro": true, "professional": true, "home": true,
	"free": true, "trial": true, "beta": true, "demo": true, "portable": true,
	"client": true, "desktop": true, "online": true, "offline": true,
	"version": true, "ver": true, "release": true,
	"программа": true, "приложение": true, "версия": true, "обновление": true,
	"установка": true, "установщик": true, "экзе": true, "ехе": true,
	"х64": true, "х86": true,
}

var tokenSynonyms = map[string]string{
	"хром": "chrome", "хромы": "chrome", "хрома": "chrome", "хроме": "chrome", "хрому": "chrome",
	"гугл": "google", "гугла": "google", "гугле": "google",
	"майкрософт": "microsoft", "майкрософта": "microsoft",
	"виндовс": "windows", "виндоус": "windows",
	"офис": "office", "офиса": "office",
	"эксель": "excel", "экселя": "excel",
	"ворд": "word", "ворда": "word",
	"пауэрпоинт": "powerpoint",
	"адоб":       "adobe", "фотошоп": "photoshop",
	"зум": "zoom", "слак": "slack", "нотпад": "notepad",
	"линукс": "linux", "антивирус": "antivirus",
}

// Normalize turns dirty inventory names into a comparable token string. It is
// deliberately conservative: version numbers, installers and architecture
// suffixes are removed, but product words and vendor names are kept.
func Normalize(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "ё", "е")
	value = punctuation.Replace(value)
	fields := strings.Fields(value)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if replacement, ok := tokenSynonyms[field]; ok {
			field = replacement
		}
		if field == "" || noiseTokens[field] || versionPattern.MatchString(field) {
			continue
		}
		if len([]rune(field)) < 2 {
			continue
		}
		result = append(result, field)
	}
	return strings.Join(result, " ")
}

func Tokens(normalized string) []string {
	if normalized == "" {
		return nil
	}
	return strings.Fields(normalized)
}

func jaccard(left, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	leftSet := make(map[string]bool, len(left))
	for _, value := range left {
		leftSet[value] = true
	}
	intersection := 0
	for _, value := range right {
		if leftSet[value] {
			intersection++
		}
	}
	union := len(leftSet) + len(right)
	for _, value := range right {
		if leftSet[value] {
			union--
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func hasPrefixOverlap(left, right []string) bool {
	for _, a := range left {
		for _, b := range right {
			if commonPrefixLength(a, b) >= 5 {
				return true
			}
		}
	}
	return false
}

func commonPrefixLength(left, right string) int {
	a := []rune(left)
	b := []rune(right)
	size := min(len(a), len(b))
	for index := 0; index < size; index++ {
		if a[index] != b[index] {
			return index
		}
	}
	return size
}

func levenshtein(left, right string) int {
	a := []rune(left)
	b := []rune(right)
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			deletion := previous[j] + 1
			insertion := current[j-1] + 1
			substitution := previous[j-1] + cost
			current[j] = min(deletion, insertion, substitution)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func levenshteinRatio(left, right string) float64 {
	if left == "" && right == "" {
		return 1
	}
	distance := levenshtein(left, right)
	maximum := max(len([]rune(left)), len([]rune(right)))
	if maximum == 0 {
		return 1
	}
	return 1 - float64(distance)/float64(maximum)
}

func scorePair(rawNormalized string, rawTokens []string, candidateNormalized string, candidateTokens []string) (float64, string) {
	if rawNormalized == "" || candidateNormalized == "" {
		return 0, ""
	}
	if rawNormalized == candidateNormalized {
		return 1, "exact"
	}
	if strings.Contains(rawNormalized, candidateNormalized) || strings.Contains(candidateNormalized, rawNormalized) {
		return 0.93, "token"
	}
	best := 0.0
	method := ""
	overlap := jaccard(rawTokens, candidateTokens)
	if overlap > 0 {
		score := 0.55 + 0.4*overlap
		if score > best {
			best, method = score, "token"
		}
	}
	if hasPrefixOverlap(rawTokens, candidateTokens) {
		score := 0.72 + 0.1*overlap
		if score > best {
			best, method = score, "token"
		}
	}
	ratio := levenshteinRatio(rawNormalized, candidateNormalized)
	if ratio >= 0.6 {
		score := 0.5 + 0.45*ratio
		if score > best {
			best, method = score, "fuzzy"
		}
	}
	return best, method
}

type productIndex struct {
	id         int64
	name       string
	normalized string
	tokens     []string
	aliases    []aliasIndex
}

type aliasIndex struct {
	alias      string
	normalized string
	tokens     []string
	confidence float32
}

type Match struct {
	ProductID   int64
	ProductName string
	Score       float64
	Method      string
}

func BuildIndex(products []CatalogProduct, aliases []CatalogAlias) []productIndex {
	index := make([]productIndex, 0, len(products))
	byID := make(map[int64]int, len(products))
	for _, product := range products {
		normalized := Normalize(product.Name)
		index = append(index, productIndex{
			id:         product.ID,
			name:       product.Name,
			normalized: normalized,
			tokens:     Tokens(normalized),
		})
		byID[product.ID] = len(index) - 1
	}
	for _, alias := range aliases {
		position, ok := byID[alias.ProductID]
		if !ok {
			continue
		}
		normalized := Normalize(alias.Alias)
		if normalized == "" {
			continue
		}
		index[position].aliases = append(index[position].aliases, aliasIndex{
			alias:      alias.Alias,
			normalized: normalized,
			tokens:     Tokens(normalized),
			confidence: alias.Confidence,
		})
	}
	return index
}

func BestMatches(rawName string, index []productIndex, limit int) []Match {
	rawNormalized := Normalize(rawName)
	rawTokens := Tokens(rawNormalized)
	if rawNormalized == "" {
		return nil
	}
	bestByProduct := make(map[int64]Match)
	for _, product := range index {
		bestScore, bestMethod := scorePair(rawNormalized, rawTokens, product.normalized, product.tokens)
		for _, alias := range product.aliases {
			score, method := scorePair(rawNormalized, rawTokens, alias.normalized, alias.tokens)
			if alias.confidence < 1 {
				score *= float64(alias.confidence)
			}
			if score > bestScore {
				bestScore, bestMethod = score, method
			}
		}
		if bestScore < matchThreshold {
			continue
		}
		current, exists := bestByProduct[product.id]
		if !exists || bestScore > current.Score {
			bestByProduct[product.id] = Match{ProductID: product.id, ProductName: product.name, Score: bestScore, Method: bestMethod}
		}
	}
	result := make([]Match, 0, len(bestByProduct))
	for _, match := range bestByProduct {
		result = append(result, match)
	}
	sortMatches(result)
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func sortMatches(matches []Match) {
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0; j-- {
			if matches[j].Score > matches[j-1].Score ||
				(matches[j].Score == matches[j-1].Score && matches[j].ProductID < matches[j-1].ProductID) {
				matches[j], matches[j-1] = matches[j-1], matches[j]
				continue
			}
			break
		}
	}
}

// CatalogProduct is the minimal product shape used by the matcher.
type CatalogProduct struct {
	ID   int64
	Name string
}

// CatalogAlias is the minimal alias shape used by the matcher.
type CatalogAlias struct {
	ProductID  int64
	Alias      string
	Confidence float32
}
