package matcher

import (
	"strings"
	"unicode"

	"github.com/justsky/spotsonic/internal/subsonic"
)

// BestMatch returns the highest-scoring song from candidates.
// Scoring: title similarity * 0.6 + artist similarity * 0.4.
// Returns (song, score, found). found is true only when score >= threshold.
func BestMatch(songs []subsonic.Song, title, artist string, threshold float64) (subsonic.Song, float64, bool) {
	normTitle := normalize(title)
	normArtist := normalize(artist)

	var bestSong subsonic.Song
	var bestScore float64

	for _, s := range songs {
		ts := similarity(normalize(s.Title), normTitle)
		as := similarity(normalize(s.Artist), normArtist)
		score := ts*0.6 + as*0.4
		if score > bestScore {
			bestScore = score
			bestSong = s
		}
	}

	return bestSong, bestScore, bestScore >= threshold
}

// normalize lowercases s, strips non-alphanumeric characters, and collapses spaces.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// similarity returns a 0–1 score between two pre-normalized strings.
func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}
	// containment: short string fully inside longer → high partial score
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter, longer := len(a), len(b)
		if longer < shorter {
			shorter, longer = longer, shorter
		}
		return float64(shorter) / float64(longer)
	}
	dist := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshtein computes the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			if ra[i-1] == rb[j-1] {
				curr[j] = prev[j-1]
			} else {
				curr[j] = 1 + min3(prev[j], curr[j-1], prev[j-1])
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
