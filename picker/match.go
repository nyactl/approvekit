// Package picker lets a person choose one of many lines in the terminal by
// typing part of it, the way fzf does: every word of the query must occur
// in a line in order, letters may be skipped, and the best matches come
// first. It also shows longer text full screen.
//
// Lines are sanitized before they are drawn, like all output of the tools.
package picker

import (
	"sort"
	"strings"
	"unicode"
)

// score rates how well text matches one word of a query, or reports that
// it does not match: every rune of word must occur in text in order,
// ignoring case. Runes in a row, at the start of a word and early in the
// text score higher.
func score(word, text string) (int, bool) {
	w := []rune(strings.ToLower(word))
	t := []rune(text)
	if len(w) == 0 {
		return 0, true
	}
	best, found := 0, false
	// Try each place where the first rune occurs, keeping the best.
	for start := range t {
		if unicode.ToLower(t[start]) != w[0] {
			continue
		}
		s, ok := scoreFrom(w, t, start)
		if ok && (!found || s > best) {
			best, found = s, true
		}
	}
	return best, found
}

func scoreFrom(w, t []rune, start int) (int, bool) {
	s, wi, prev := 0, 0, -2
	for ti := start; ti < len(t) && wi < len(w); ti++ {
		if unicode.ToLower(t[ti]) != w[wi] {
			continue
		}
		s += 10
		if ti == prev+1 {
			s += 15
		}
		if ti == 0 || !isAlnum(t[ti-1]) {
			s += 20
		}
		prev = ti
		wi++
	}
	if wi < len(w) {
		return 0, false
	}
	// Earlier matches are better, and a whole line matched is best.
	s -= start / 4
	if len(w) == len(t) {
		s += 50
	}
	return s, true
}

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// Filter returns the indexes of the items that match every word of query,
// best first; equal scores keep the items' order. An empty query keeps all.
func Filter(query string, items []string) []int {
	words := strings.Fields(query)
	type hit struct{ i, s int }
	hits := make([]hit, 0, len(items))
	for i, it := range items {
		total, ok := 0, true
		for _, w := range words {
			s, m := score(w, it)
			if !m {
				ok = false
				break
			}
			total += s
		}
		if ok {
			hits = append(hits, hit{i, total})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].s > hits[b].s })
	out := make([]int, len(hits))
	for k, h := range hits {
		out[k] = h.i
	}
	return out
}
