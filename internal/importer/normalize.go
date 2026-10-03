package importer

import (
	"strings"
	"unicode"
)

var streetWords = map[string]string{
	"street": "st", "avenue": "ave", "av": "ave", "place": "pl", "road": "rd",
	"boulevard": "blvd", "lane": "ln", "drive": "dr", "square": "sq", "court": "ct",
	"terrace": "ter", "plaza": "plz", "parkway": "pkwy", "highway": "hwy",
	"west": "w", "east": "e", "north": "n", "south": "s",
}

var ordinalWords = map[string]string{
	"first": "1", "second": "2", "third": "3", "fourth": "4", "fifth": "5", "sixth": "6",
	"seventh": "7", "eighth": "8", "ninth": "9", "tenth": "10", "eleventh": "11", "twelfth": "12",
}

// Tokens that introduce a unit designator; they and the token after them are dropped.
var unitWords = map[string]bool{"apt": true, "unit": true, "ste": true, "suite": true, "fl": true, "floor": true, "rm": true, "room": true}

// NormalizeAddress reduces a street address to a comparable key, so
// "75 Seventh Avenue South, Apt 2" and "75 7th Ave S" compare equal.
func NormalizeAddress(addr string) string {
	addr = strings.ToLower(addr)
	// Only the street line matters: drop city, state and ZIP after the first comma.
	if i := strings.IndexByte(addr, ','); i >= 0 {
		addr = addr[:i]
	}
	addr = strings.ReplaceAll(addr, "avenue of the americas", "6 ave")

	fields := strings.FieldsFunc(addr, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '#'
	})
	out := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if unitWords[f] {
			i++ // skip the unit number too
			continue
		}
		if strings.HasPrefix(f, "#") {
			continue
		}
		if w, ok := ordinalWords[f]; ok {
			f = w
		} else if w, ok := streetWords[f]; ok {
			f = w
		} else {
			f = stripOrdinal(f)
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// stripOrdinal turns "7th" into "7" and leaves anything else alone.
func stripOrdinal(s string) string {
	for _, suf := range []string{"st", "nd", "rd", "th"} {
		if n := strings.TrimSuffix(s, suf); n != s && n != "" && isDigits(n) {
			return n
		}
	}
	return s
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Similarity is pg_trgm's similarity(): the Jaccard index of the two strings' trigram sets,
// where each lower-cased alphanumeric word is padded with two spaces in front and one behind.
func Similarity(a, b string) float64 {
	ta, tb := trigrams(a), trigrams(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	shared := 0
	for t := range ta {
		if tb[t] {
			shared++
		}
	}
	return float64(shared) / float64(len(ta)+len(tb)-shared)
}

func trigrams(s string) map[string]bool {
	set := map[string]bool{}
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		padded := []rune("  " + w + " ")
		for i := 0; i+3 <= len(padded); i++ {
			set[string(padded[i:i+3])] = true
		}
	}
	return set
}
