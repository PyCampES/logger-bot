// Package extraction parses workout data (exercise / reps / weight / category / unit)
// from a free-text transcription. Bilingual (Spanish + English keywords).
//
// Always returns a fully-populated Workout — never errors. Falls back to
// "Unknown Exercise" and zeroed numeric fields rather than failing, because
// downstream handlers and the SQLite schema rely on every field being present.
package extraction

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Workout struct {
	Category string
	Exercise string
	Reps     int
	Weight   float64
	Unit     string
	RawText  string
}

var (
	weightRe     = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(kg|kilos?|lbs?|pounds?|libras?)`)
	repsRe       = regexp.MustCompile(`(\d+)\s*(repeticiones?|repetitions?|reps?)`)
	categoryRe   = regexp.MustCompile(`categor[ií]a[\s,]+(?:de\s+)?(\w+)`)
	porLadoRe    = regexp.MustCompile(`por\s+lado`)
	spanishNumRe = regexp.MustCompile(`(cuatro|cinco|seis|siete|ocho|nueve|diez)\s+repeticiones?`)
	conRe        = regexp.MustCompile(`\bcon\b`)
	punctRe      = regexp.MustCompile(`[^\w\s]`)
	wsRe         = regexp.MustCompile(`\s+`)

	titleCaser = cases.Title(language.Und)
)

func normalizeUnit(raw string) string {
	switch raw {
	case "kilo", "kilos":
		return "kg"
	case "lb", "lbs", "pound", "pounds", "libra", "libras":
		return "lbs"
	default:
		return raw
	}
}

// Parse extracts workout data from a transcription. Mirrors the Python parser
// in src/logger_bot/extraction.py exactly.
func Parse(text string) Workout {
	w := Workout{Unit: "kg", RawText: text}
	low := strings.ToLower(text)

	// 1. Weight + unit
	if m := weightRe.FindStringSubmatchIndex(low); m != nil {
		full := low[m[0]:m[1]]
		num, _ := strconv.ParseFloat(low[m[2]:m[3]], 64)
		w.Weight = num
		w.Unit = normalizeUnit(low[m[4]:m[5]])
		low = strings.Replace(low, full, " ", 1)
	}

	// 2. Reps (numeric form only — Spanish word-numbers are stripped, not parsed)
	if m := repsRe.FindStringSubmatchIndex(low); m != nil {
		full := low[m[0]:m[1]]
		n, _ := strconv.Atoi(low[m[2]:m[3]])
		w.Reps = n
		low = strings.Replace(low, full, " ", 1)
	}

	// 3. Category
	if m := categoryRe.FindStringSubmatchIndex(low); m != nil {
		full := low[m[0]:m[1]]
		w.Category = titleCaser.String(low[m[2]:m[3]])
		low = strings.Replace(low, full, " ", 1)
	}

	// 4. Build exercise from what's left
	low = porLadoRe.ReplaceAllString(low, "")
	low = spanishNumRe.ReplaceAllString(low, "")
	low = conRe.ReplaceAllString(low, "")
	low = punctRe.ReplaceAllString(low, " ")
	low = wsRe.ReplaceAllString(strings.TrimSpace(low), " ")

	if low == "" {
		w.Exercise = "Unknown Exercise"
	} else {
		w.Exercise = titleCaser.String(low)
	}
	return w
}