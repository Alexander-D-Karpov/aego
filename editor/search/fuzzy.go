package search

const (
	neg          = -1 << 30
	bonusStart   = 14
	bonusWord    = 10
	bonusCamel   = 9
	bonusConsec  = 6
	bonusCase    = 1
	bonusPrefix  = 20
	gapPenalty   = 1
	leadPenalty  = 3
	lengthWeight = 8
)

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func upper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func separator(c byte) bool {
	switch c {
	case '/', '.', '_', '-', ' ', ':':
		return true
	}
	return false
}

func bonus(s string, i int) int {
	switch {
	case i == 0:
		return bonusStart
	case separator(s[i-1]):
		return bonusWord
	case upper(s[i]) && !upper(s[i-1]):
		return bonusCamel
	}
	return 0
}

func Match(pattern, s string, pos []int) (int, []int, bool) {
	pos = pos[:0]
	m, n := len(pattern), len(s)
	if m == 0 {
		return 0, pos, true
	}
	if m > n {
		return 0, pos, false
	}
	prev := make([]int, n)
	cur := make([]int, n)
	from := make([]int, m*n)
	for i := 0; i < n; i++ {
		prev[i] = neg
		if lower(s[i]) == lower(pattern[0]) {
			prev[i] = bonus(s, i) - min(i, leadPenalty)
			if s[i] == pattern[0] {
				prev[i] += bonusCase
			}
		}
	}
	for j := 1; j < m; j++ {
		g, gk := neg, -1
		for i := 0; i < n; i++ {
			if i >= 2 {
				g -= gapPenalty
				if v := prev[i-2] - gapPenalty; v > g {
					g, gk = v, i-2
				}
			}
			cur[i] = neg
			if lower(s[i]) != lower(pattern[j]) {
				continue
			}
			best, bk := g, gk
			if i >= 1 && prev[i-1] > neg && prev[i-1]+bonusConsec >= best {
				best, bk = prev[i-1]+bonusConsec, i-1
			}
			if best <= neg/2 {
				continue
			}
			cur[i] = best + bonus(s, i)
			if s[i] == pattern[j] {
				cur[i] += bonusCase
			}
			from[j*n+i] = bk
		}
		prev, cur = cur, prev
	}
	end, score := -1, neg
	for i := 0; i < n; i++ {
		if prev[i] > score {
			end, score = i, prev[i]
		}
	}
	if end < 0 || score <= neg/2 {
		return 0, pos, false
	}
	for k := 0; k < m; k++ {
		pos = append(pos, 0)
	}
	for j, i := m-1, end; j >= 0; j-- {
		pos[j] = i
		if j > 0 {
			i = from[j*n+i]
		}
	}
	if pos[0] == 0 {
		score += bonusPrefix
	}
	return score - n/lengthWeight, pos, true
}
