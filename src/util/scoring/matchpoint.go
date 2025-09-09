package scoring

import (
	"sort"
	"fmt"
	"src/types"
)


type Tournament struct {
	Id string
	BoardsPerRound int
	TotalRounds int
	Type int
	Teams int 
}

func CalculateMatchpoints(results []types.BoardResult) map[string]types.MatchpointScore {
	type scoredResult struct {
		PairID    string
		Direction string
		RawScore  int
		Contract string
		ContractDirection string
		Result string
	}
	var scoredResults []scoredResult

	// Only use NS side as the reference score
	for _, res := range results {
		raw := res.Score
		scoredResults = append(scoredResults, scoredResult{
			PairID:    res.NSPairId,
			Direction: "NS",
			RawScore:  raw,
			Contract: res.Contract,
			ContractDirection: res.Direction,
			Result: res.Result,
		})
	}

	// Sort by descending score
	sort.SliceStable(scoredResults, func(i, j int) bool {
		return scoredResults[i].RawScore > scoredResults[j].RawScore
	})

	mpScores := make(map[string]types.MatchpointScore)
	n := len(scoredResults)

	if n == 1 {
		// Only one result exists; assign 0 matchpoints to both NS and EW
		nsPair := scoredResults[0].PairID
		nsScore := scoredResults[0].RawScore

		var ewPair string
		for _, res := range results {
			if res.NSPairId == nsPair {
				ewPair = res.EWPairId
				break
			}
		}

		mpScores[nsPair] = types.MatchpointScore{
			PairID:    nsPair,
			Direction: "NS",
			MPScore:   0.5,
			RawScore:  nsScore,
			Contract: scoredResults[0].Contract,
			ContractDirection: scoredResults[0].ContractDirection,
			Result: scoredResults[0].Result,
		}
		mpScores[ewPair] = types.MatchpointScore{
			PairID:    ewPair,
			Direction: "EW",
			MPScore:   0.5,
			RawScore:  -nsScore,
			Contract: scoredResults[0].Contract,
			ContractDirection: scoredResults[0].ContractDirection,
			Result: scoredResults[0].Result,
		}
		fmt.Printf("MP scores %s: raw=%d ,mps=%f\n",nsPair,mpScores[nsPair].RawScore,mpScores[nsPair].MPScore)
		fmt.Printf("MP scores %s: raw=%d ,mps=%f\n",ewPair,mpScores[ewPair].RawScore,mpScores[ewPair].MPScore)
		return mpScores
	}

	// Normal multi-entry matchpoint scoring
	for i := range scoredResults {
		mp := 0.0
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			if scoredResults[i].RawScore > scoredResults[j].RawScore {
				mp += 1
			} else if scoredResults[i].RawScore == scoredResults[j].RawScore {
				mp += 0.5
			}
		}

		nsPair := scoredResults[i].PairID
		ewPair := ""

		// Find matching EW pair from original result
		for _, res := range results {
			if res.NSPairId == nsPair {
				ewPair = res.EWPairId
				break
			}
		}

		// Store MP for NS
		mpScores[nsPair] = types.MatchpointScore{
			PairID:    nsPair,
			Direction: "NS",
			MPScore:   mp,
			RawScore:  scoredResults[i].RawScore,
			Contract: scoredResults[i].Contract,
			ContractDirection: scoredResults[i].ContractDirection,
			Result: scoredResults[i].Result,
		}

		// MP for EW is inverse (max = n - 1)
		ewMP := float64(n-1) - mp
		mpScores[ewPair] = types.MatchpointScore{
			PairID:    ewPair,
			Direction: "EW",
			MPScore:   ewMP,
			RawScore:  -scoredResults[i].RawScore,
			Contract: scoredResults[i].Contract,
			ContractDirection: scoredResults[i].ContractDirection,
			Result: scoredResults[i].Result,
		}
		fmt.Printf("MP scores %s: raw=%d ,mps=%f\n",nsPair,mpScores[nsPair].RawScore,mpScores[nsPair].MPScore)
		fmt.Printf("MP scores %s: raw=%d ,mps=%f\n",ewPair,mpScores[ewPair].RawScore,mpScores[ewPair].MPScore)
	}

	return mpScores
}




