package scoring

import (
	"strconv"
	"strings"
)

func CalculateScore(contract string, direction string, result string, generalVul int) int {
	// Example contract: "4HX" or "3NTXX"
	level, _ := strconv.Atoi(string(contract[0]))
	rest := contract[1:]
	vul := 0

	if generalVul == 3 || (generalVul == 1 && direction == "NS") || (generalVul == 2 && direction == "EW"){
		vul = 1
	}
	

	// Extract denomination and double/redouble
	denom := ""
	doubleType := "" // "", "X", "XX"

	for _, r := range []string{"NT", "S", "H", "D", "C"} {
		if strings.HasPrefix(rest, r) {
			denom = r
			doubleType = strings.TrimPrefix(rest, r)
			break
		}
	}

	// Trick point values
	basePoints := 0
	switch denom {
		case "C", "D":
			basePoints = 20
		case "H", "S":
			basePoints = 30
		case "NT":
			basePoints = 30
	}

	multiplier := 1
	if doubleType == "X" {
		multiplier = 2
	} else if doubleType == "XX" {
		multiplier = 4
	}

	// Parse result
	overUnder := 0
	contractTricks := level + 6
	result = strings.TrimSpace(result)
	if result == "=" {
		overUnder = 0
	} else if strings.HasPrefix(result, "+") || strings.HasPrefix(result, "-") {
		sign := result[0]
		num, _ := strconv.Atoi(result[1:])
		if sign == '+' {
			overUnder = num
		} else if sign == '-' {
			overUnder = -num
		} 
	} else {
		num, _ := strconv.Atoi(result)
		overUnder = num - level
	}

	totalTricks := contractTricks + overUnder

	score := 0

	if totalTricks >= contractTricks {
		// Contract made
		trickPoints := 0
		if denom == "NT" {
			trickPoints = 40 + (level-1)*30
		} else {
			trickPoints = level * basePoints
		}

		trickPoints *= multiplier
		score += trickPoints

		// Bonuses
		if multiplier > 1 {
			score += 50 // insult bonus for (re)doubled
		}

		// Game or part-score
		if trickPoints >= 100 {
			if vul == 1 {
				score += 500
			} else {
				score += 300
			}
		} else {
			score += 50
		}

		// Slam bonuses
		if level == 6 {
			if vul == 1 {
				score += 750
			} else {
				score += 500
			}
		} else if level == 7 {
			if vul == 1 {
				score += 1500
			} else {
				score += 1000
			}
		}

		// Overtricks
		if overUnder > 0 {
			if multiplier == 1 {
				score += overUnder * basePoints
			} else if multiplier == 2 {
				if vul == 1 {
					score += overUnder * 200
				} else {
					score += overUnder * 100
				}
			} else if multiplier == 4 {
				if vul == 1 {
					score += overUnder * 400
				} else {
					score += overUnder * 200
				}
			}
		}
	} else {
		// Contract went down
		tricksDown := -overUnder
		if multiplier == 1 {
			if vul == 1 {
				score -= 100 * tricksDown
			} else {
				score -= 50 * tricksDown
			}
		} else {
			// Doubled or redoubled undertricks
			base := 100
			if vul == 0 {
				if tricksDown == 1 {
					base = 100
				} else if tricksDown == 2 {
					base = 100 + 200
				} else if tricksDown >= 3 {
					base = 100 + 200 + (tricksDown-2)*300
				}
			} else {
				if tricksDown == 1 {
					base = 200
				} else if tricksDown >= 2 {
					base = 200 + (tricksDown-1)*300
				}
			}
			if multiplier == 4 {
				score -= base * 2 // redoubled
			} else {
				score -= base
			}
		}
	}

	return score
}

