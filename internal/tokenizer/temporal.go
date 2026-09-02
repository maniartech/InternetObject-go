package tokenizer

import "time"

// Temporal literal validation and decoding (io-specs date-and-time.md).
//
// The three kinds stay distinct all the way out: a date is not a datetime
// with a zero time, and flattening them loses the author's intent. Validation
// is calendar-strict — d'2024-02-30' is invalid-date, not normalized.

// temporal holds the parsed components of one temporal literal.
type temporal struct {
	y, mo, d   int
	h, mi, sec int
	ms         int
	offMin     int  // timezone offset in minutes (datetime only)
	hasOff     bool // an explicit offset or Z was written
}

// parseTemporal validates the quoted content of a temporal literal for its
// kind and returns the components.
func parseTemporal(s string, sub Sub) (temporal, bool) {
	var t temporal
	t.mo, t.d = 1, 1
	switch sub {
	case SubDate:
		n, ok := parseDatePrefix(s, &t)
		return t, ok && n == len(s)
	case SubTime:
		return t, parseTimeString(s, &t)
	default: // SubDateTime: dateContent ["T" timeContent] [timeZone]
		n, ok := parseDatePrefix(s, &t)
		if !ok {
			return t, false
		}
		rest := s[n:]
		if rest != "" {
			if rest[0] == 'T' {
				rest = rest[1:]
				// the zone begins at the first Z, + or - (time content is only
				// digits, colons and a dot)
				zi := len(rest)
				for i := 0; i < len(rest); i++ {
					if rest[i] == 'Z' || rest[i] == '+' || rest[i] == '-' {
						zi = i
						break
					}
				}
				if !parseTimeString(rest[:zi], &t) {
					return t, false
				}
				rest = rest[zi:]
			}
			if rest != "" && !parseZone(rest, &t) {
				return t, false
			}
		}
		// A nonzero offset can carry the UTC instant outside the four-digit
		// years the wire format can spell (0000 with +01:00 lands in year
		// -1); such a datetime is invalid rather than unwritable. The
		// reference accepts it and then emits `-000001-…`, which its own
		// reader rejects — an upstream finding, not behavior to reproduce.
		if t.hasOff && t.offMin != 0 {
			if y := t.timeValue(SubDateTime).UTC().Year(); y < 0 || y > 9999 {
				return t, false
			}
		}
		return t, true
	}
}

// parseDatePrefix reads dateContent — YYYY, then an optional month and day,
// each with an optional hyphen — from the front of s. It returns how many
// bytes it consumed; the caller decides whether a remainder is legal.
func parseDatePrefix(s string, t *temporal) (int, bool) {
	if len(s) < 4 || !allDigits(s[:4]) {
		return 0, false
	}
	t.y = atoi(s[:4])
	i := 4
	for _, dst := range []*int{&t.mo, &t.d} {
		j := i
		if j < len(s) && s[j] == '-' {
			j++
		}
		if j+2 > len(s) || !allDigits(s[j:j+2]) {
			break
		}
		*dst = atoi(s[j : j+2])
		i = j + 2
	}
	if t.mo < 1 || t.mo > 12 || t.d < 1 || t.d > daysIn(t.y, t.mo) {
		return 0, false
	}
	return i, true
}

// parseTimeString reads timeContent, whole-string: HH, then optional minute,
// second (each with an optional colon) and an optional .SSS with exactly
// three digits.
func parseTimeString(s string, t *temporal) bool {
	if len(s) < 2 || !allDigits(s[:2]) {
		return false
	}
	t.h = atoi(s[:2])
	i := 2
	for _, dst := range []*int{&t.mi, &t.sec} {
		j := i
		if j < len(s) && s[j] == ':' {
			j++
		}
		if j+2 > len(s) || !allDigits(s[j:j+2]) {
			break
		}
		*dst = atoi(s[j : j+2])
		i = j + 2
	}
	if i < len(s) {
		if s[i] != '.' || len(s)-i-1 != 3 || !allDigits(s[i+1:]) {
			return false
		}
		t.ms = atoi(s[i+1:])
		i = len(s)
	}
	return t.h <= 23 && t.mi <= 59 && t.sec <= 59
}

// parseZone reads a whole-string timeZone: Z, or a signed offset in ±HH,
// ±HH:mm or ±HHMM form, range -12:00 to +14:00.
func parseZone(s string, t *temporal) bool {
	t.hasOff = true
	if s == "Z" {
		return true
	}
	if len(s) < 3 || (s[0] != '+' && s[0] != '-') || !allDigits(s[1:3]) {
		return false
	}
	h := atoi(s[1:3])
	m := 0
	switch {
	case len(s) == 3:
	case len(s) == 5 && allDigits(s[3:5]):
		m = atoi(s[3:5])
	case len(s) == 6 && s[3] == ':' && allDigits(s[4:6]):
		m = atoi(s[4:6])
	default:
		return false
	}
	if m > 59 {
		return false
	}
	t.offMin = h*60 + m
	if s[0] == '-' {
		t.offMin = -t.offMin
	}
	return t.offMin >= -12*60 && t.offMin <= 14*60
}

func daysIn(y, mo int) int {
	switch mo {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	}
	if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
		return 29
	}
	return 28
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// timeValue converts parsed components to a time.Time. A date is UTC
// midnight; a time uses the reference date 1900-01-01 UTC; a datetime with an
// explicit offset keeps it (fixed zone), otherwise UTC.
func (t temporal) timeValue(sub Sub) time.Time {
	loc := time.UTC
	if t.hasOff && t.offMin != 0 {
		loc = time.FixedZone("", t.offMin*60)
	}
	switch sub {
	case SubDate:
		return time.Date(t.y, time.Month(t.mo), t.d, 0, 0, 0, 0, time.UTC)
	case SubTime:
		return time.Date(1900, 1, 1, t.h, t.mi, t.sec, t.ms*1e6, time.UTC)
	default:
		return time.Date(t.y, time.Month(t.mo), t.d, t.h, t.mi, t.sec, t.ms*1e6, loc)
	}
}
