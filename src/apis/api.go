package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"src/util"
	"github.com/redis/go-redis/v9"
	"strconv"
	"src/util/scoring"
	"math"
	"src/types"
)

const (
	PairGame = iota
	TeamGame
)

const (
	VulNone = iota
	VulNS
	VulEW
	VulAll
)

type Handler struct {
	Redis *redis.Client
	WebSocketHub *WebSocketHub
	ExpectedClients map[string]int
}

type Tournament struct {
	Id string
	BoardsPerRound int
	TotalRounds int
	Type int
	Teams int  //# of Pairs if pair game
}

type Pair struct {
	Id string  //1NS or 4EW
	Name1 string
	Name2 string 
	TournamentId string
}

type BoardState struct {
	CurrentBoard int
	CurrentOpp string
	CurrentRound int
}

type PairStateResponse struct {
	PairId string
	BoardState *BoardState
	IsOver bool
}

type SortedResult struct {
	PairId string
	Name1 string
	Name2 string
	Score types.MatchpointScore
}

type PairResultByBoard struct{
	BoardNumber int
	Contract string
	Result string
	Direction string
	RawScore int
	Percentage float64
}

func CalculateLeaderboard(h *Handler, ctx context.Context,allResults []types.BoardResult,tournament Tournament,tournamentId string) (map[string]types.MatchpointScore,error) {
	boardResults := make(map[int][]types.BoardResult)
	var err error

	for _,res := range allResults{
		boardResults[res.BoardNumber] = append(boardResults[res.BoardNumber],res)
	}

	totalScores := make(map[string]types.MatchpointScore)
	pairResultByBoard := make(map[string]map[int]PairResultByBoard)
	for boardNumber,results := range boardResults{
		var maxMPs float64
		mpScores := scoring.CalculateMatchpoints(results)
		if len(results) == 1{
			maxMPs = 1
		} else{
			maxMPs = float64(len(results)-1)
		}
		for pairId,score := range mpScores{
			if mpScore,ok := totalScores[pairId]; ok {
				mpScore.MPScore += score.MPScore
				totalScores[pairId] = mpScore
				fmt.Printf("New totalScores for pair %s is %f by adding mpscore %f\n",pairId,totalScores[pairId].MPScore,score.MPScore)
			} else{
				totalScores[pairId] = score
				fmt.Printf("New totalScores for pair %s is %f\n",pairId,totalScores[pairId].MPScore)
			}
			if _,ok := pairResultByBoard[pairId]; !ok {
				pairResultByBoard[pairId] = make(map[int]PairResultByBoard)
			}
			var percentage float64
			if len(results) == 1{
				percentage = 50.00
			} else{
				percentage = math.Round((score.MPScore / float64(maxMPs)) * 100 * 100)/100
			}
			newPairResultByBoard := PairResultByBoard{
				BoardNumber: boardNumber,
				Contract: score.Contract,
				Result: score.Result,
				Direction: score.ContractDirection,
				RawScore: score.RawScore,
				Percentage: percentage,
			}
			pairResultByBoard[pairId][boardNumber] = newPairResultByBoard
			fmt.Printf("New Result for pair %s on board %d: %+v\n",pairId,boardNumber,newPairResultByBoard)
		}
	}

	//set pair individual board statistics to redis
	for pairId,results := range pairResultByBoard{
		key := fmt.Sprintf("tournament:%s:pair:%s:boardResults",tournamentId,pairId)
		for boardNumber,result := range results{
			fmt.Printf("Results for board number %d: %+v\n",boardNumber,result)

			data, err := json.Marshal(result)
			if err != nil {
				fmt.Println("Error marshaling")
				continue
			}
			err = h.Redis.HSet(ctx,key,fmt.Sprintf("board:%d", boardNumber),string(data)).Err()
			if err != nil {
				fmt.Println(err,key)
				continue
			}
		}
	}

	numPairs := tournament.Teams/2
	numBoards := tournament.BoardsPerRound * tournament.TotalRounds
	maxMPsPerPair := float64(numBoards) * float64(numPairs-1)

	for pairId,score := range totalScores{
		if numPairs == 1{
			score.Percentage = 0
		} else{
			score.Percentage = math.Round((score.MPScore / maxMPsPerPair) * 100 * 100)/100
		}
		fmt.Printf("Pair Id %s has %f MPs resulting in %f\n",pairId,score.MPScore,score.Percentage)
		totalScores[pairId] = score
	}

	return totalScores,err
}

func GetTournamentById(h *Handler, ctx context.Context, tournamentId string) (*Tournament, error) {
	key := fmt.Sprintf("tournament:%s",tournamentId)
	tournament,err := h.Redis.HGetAll(ctx,key).Result()	

	if err != nil {
		return nil, fmt.Errorf("failed to fetch tournament: %w", err)
	}
	if len(tournament) == 0 {
        return nil, fmt.Errorf("tournament %s not found", tournamentId)
    }

    var t Tournament

    if val, ok := tournament["BoardsPerRound"]; ok {
        fmt.Sscanf(val, "%d", &t.BoardsPerRound)
    }
	if val, ok := tournament["TotalRounds"]; ok {
        fmt.Sscanf(val, "%d", &t.TotalRounds)
    }
    if val, ok := tournament["Type"]; ok {
        fmt.Sscanf(val, "%d", &t.Type)
    }
    if val, ok := tournament["Teams"]; ok {
        fmt.Sscanf(val, "%d", &t.Teams)
    }

    return &t, nil
}

func GetBoardStateByPairId(h *Handler, ctx context.Context, tournamentId string, pairId string) (*BoardState,error){
	key := fmt.Sprintf("tournament:%s:pair:%s:state",tournamentId,pairId)
	data,err := h.Redis.HGetAll(ctx,key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch board number: %w",err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("board for pair %s not found", pairId)
	}

	var state BoardState
    if val, ok := data["CurrentBoard"]; ok {
        fmt.Sscanf(val, "%d", &state.CurrentBoard)
    }
    if val, ok := data["CurrentOpp"]; ok {
        fmt.Sscanf(val, "%s", &state.CurrentOpp)
    }
	if val, ok := data["CurrentRound"]; ok {
        fmt.Sscanf(val, "%d", &state.CurrentRound)
    }
	fmt.Printf("Got board state %+v",state)
	return &state,nil
}

func GetNamesByPairId(h *Handler, ctx context.Context, tournamentId string, pairId string) (string,string,error) {
	key := fmt.Sprintf("tournament:%s:pair:%s",tournamentId,pairId)
	data,err := h.Redis.HGetAll(ctx,key).Result()
	if err != nil {
		return "","",fmt.Errorf("error getting pair info: %w",err)
	}
	return data["Name1"],data["Name2"],nil
}

func GetVulByBoardNumber(boardNumber int) int{
	switch boardNumber % 4 {
		case 0:
			return VulAll
		case 1:
			return VulNone
		case 2:
			return VulNS
		case 3:
			return VulEW
		default:
			return VulNone
	}
}

func GetDirectionFromPairId(pairId string) (string, error) {
	if len(pairId) < 2 {
		return "", fmt.Errorf("invalid pairId: %s", pairId)
	}
	return pairId[len(pairId)-2:], nil
}

func calculateNextBoard(currentBoard int, currentRound int, boardsPerRound int, totalRounds int, myDirection string,totalPairs int) (int,int,bool,bool,bool) {
	var nextBoard int
	roundNumber := currentRound
	isOver := false
	isSkip := false

	if currentBoard % boardsPerRound != 0 {
		fmt.Println("Round not over yet")
		nextBoard = currentBoard + 1
	} else{
		roundNumber++
		if roundNumber > totalRounds{
			isOver = true
			return 0,0,false,isOver,isSkip
		} else{
			fmt.Println("Round over, rotate pairs")
			if myDirection == "NS" {
				if currentBoard + 1 > (boardsPerRound * totalRounds){
					nextBoard = (currentBoard + 1) % (boardsPerRound * totalRounds)
				} else{
					nextBoard = currentBoard + 1
				}
				
			} else if myDirection == "EW" {
				if totalPairs % 2 == 0 && currentRound == (totalRounds+1)/2 {
					nextBoard = (currentBoard + (2 * boardsPerRound) + 1) % (boardsPerRound * totalRounds)
					isSkip = true
					fmt.Println("Skip round",currentRound)
				} else{
					nextBoard = (currentBoard + boardsPerRound + 1) % (boardsPerRound * totalRounds)
				}
			}
		}
	}
	return nextBoard, roundNumber, roundNumber != currentRound, isOver, isSkip
}

func calculateNextOpp(currentOpp string, totalPairs int, boardsPerRound int, totalRounds int, myDirection string, isNewRound bool,isSkip bool) string {
	var nextOpp string

	if !isNewRound{
		fmt.Println("Round not over yet",currentOpp)
		return currentOpp
	}

	currentOppNum,err := strconv.Atoi(currentOpp[:len(currentOpp)-2])
	if err != nil {
		fmt.Println("Atoi error on opp num parsing")
	}
	currentOppDir := currentOpp[len(currentOpp)-2:]
	totalPairsInDirection := totalPairs/2

	if myDirection == "NS" {
		var nextOppNum int
		if isSkip {
			nextOppNum = (currentOppNum-2)
		} else {
			nextOppNum = (currentOppNum-1)
		}
		
		if nextOppNum == 0 {
			nextOppNum = totalPairsInDirection
		}
		nextOpp = fmt.Sprintf("%d%s",nextOppNum,currentOppDir)
	} else if myDirection == "EW" {
		var nextOppNum int
		if isSkip {
			nextOppNum = (currentOppNum+2) %  totalPairsInDirection
		} else{
			nextOppNum = (currentOppNum+1) %  totalPairsInDirection
		}
		nextOpp = fmt.Sprintf("%d%s",nextOppNum,currentOppDir)
	}
	return nextOpp
}

func broadcastResults(h *Handler,ctx context.Context,tournamentId string,tournament Tournament) error {
	var results []types.BoardResult
	var err error
	pattern := fmt.Sprintf("tournament:%s:board:*",tournamentId)
	iter := h.Redis.Scan(ctx,0,pattern,0).Iterator()

	for iter.Next(ctx){
		key := iter.Val()

		data,err := h.Redis.HGetAll(ctx,key).Result()
		if err != nil {
			return fmt.Errorf("failed to fetch board result: %w",err)
		}

		br := types.BoardResult{}
		br.BoardNumber, _ = strconv.Atoi(data["BoardNumber"])
		br.Contract = data["Contract"]
		br.Direction = data["Direction"]
		br.Result = data["Result"]
		br.NSPairId = data["NSPairId"]
		br.EWPairId = data["EWPairId"]
		br.TournamentId = data["TournamentId"]
		br.Vul = data["Vul"]
		br.Score = 0 
		if s, ok := data["Score"]; ok {
			if val, err := strconv.Atoi(s); err == nil {
				br.Score = val
			} else {
				return fmt.Errorf("invalid Score value for key %s: %w", key, err)
			}
		}
		results = append(results,br)
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("iteration error: %w", err)
	}

	leaderboard,err := CalculateLeaderboard(h,ctx,results,tournament,tournamentId)

	var nsLeaderboard []SortedResult
	var ewLeaderboard []SortedResult

	for pairId,score := range leaderboard{
		name1,name2,_ := GetNamesByPairId(h,ctx,tournamentId,pairId)
		if pairId[1:] == "NS" {
			nsLeaderboard = append(nsLeaderboard,SortedResult{
				pairId,
				name1,
				name2,
				score,
			}) 
		} else if pairId[1:] == "EW" {
			ewLeaderboard = append(ewLeaderboard,SortedResult{
				pairId,
				name1,
				name2,
				score,
			})
		}
		fmt.Printf("Pair %s finished with a score of %f!\n",pairId,score.Percentage)
	}

	fmt.Printf("NS Results: %+v\n",nsLeaderboard)
	fmt.Printf("EW Results: %+v\n",ewLeaderboard)

	h.WebSocketHub.Broadcast(tournamentId,map[string]interface{}{
		"Type": "Results",
		"NS": nsLeaderboard,
		"EW": ewLeaderboard,
	})

	
	return err
}

func NextState(h *Handler,ctx context.Context,tournamentId string, pairId string) (*BoardState,error,bool) {
	isOver := false
	boardState,err := GetBoardStateByPairId(h,ctx,tournamentId,pairId)
	if err != nil {
		return nil,fmt.Errorf("unable to get board state %w",err),isOver
	}

	tournament,err := GetTournamentById(h,ctx,tournamentId)
	if err != nil {
		return nil,fmt.Errorf("unable to get tournament %w",err),isOver
	}
	boardsPerRound := tournament.BoardsPerRound
	totalRounds := tournament.TotalRounds
	totalPairs := tournament.Teams

	myDirection,err := GetDirectionFromPairId(pairId)
	if err != nil {
		return nil,fmt.Errorf("unable to get my direction %w",err),isOver
	}

	nextBoard,currentRound,isNewRound,isOver,isSkip := calculateNextBoard(boardState.CurrentBoard, boardState.CurrentRound, boardsPerRound, totalRounds, myDirection,totalPairs)
	if isOver{
		fmt.Println("Tournament has ended for pair",pairId)
		finishedKey := fmt.Sprintf("tournament:%s:pairs_finished_counter",tournamentId)
		finishedCount,err := h.Redis.Incr(ctx,finishedKey).Result()
		if err != nil {
			return nil,fmt.Errorf("unable to increment %w",err),isOver
		}
		if finishedCount == int64(tournament.Teams) {
			broadcastResults(h,ctx,tournamentId,*tournament)
		}
		return nil,nil,isOver
	}
	fmt.Println("Next board is",nextBoard,"for pair",pairId)
	nextOpp := calculateNextOpp(boardState.CurrentOpp, totalPairs, boardsPerRound, totalRounds, myDirection, isNewRound,isSkip)
	fmt.Println("Next opp is",nextOpp,"for pair",pairId)
	
	fmt.Println("Next round is",currentRound)

	boardStateKey := fmt.Sprintf("tournament:%s:pair:%s:state",tournamentId,pairId)
	err = h.Redis.HSet(ctx,boardStateKey,map[string]interface{}{
		"CurrentBoard":nextBoard,
		"CurrentRound":currentRound,
		"CurrentOpp":nextOpp,
	}).Err()
	if err != nil {
		return nil,fmt.Errorf("unable to update state %w",err),isOver
	}
	newBoardState := &BoardState{
		CurrentBoard:nextBoard,
		CurrentOpp:nextOpp,
		CurrentRound:currentRound,
	}

	return newBoardState,nil,isOver
}

func withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow requests from frontend origin
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler(w, r)
	}
}

func Routes(mux *http.ServeMux, redisCli *redis.Client){
	h := &Handler{
		Redis: redisCli,
		WebSocketHub:NewWebSocketHub(),
	}

	mux.HandleFunc("/tournament", withCORS(h.TournamentHandler))
	mux.HandleFunc("/pair", withCORS(h.PairHandler))
	mux.HandleFunc("/board", withCORS(h.BoardHandler))
	mux.HandleFunc("/pairresults",withCORS(h.PairResultsHandler))

	mux.HandleFunc("/ws",h.WsHandler)
}

func (h *Handler) TournamentHandler(w http.ResponseWriter, r *http.Request){
	fmt.Println("Handle Tournament",r.Method)
	ctx := r.Context()

	switch r.Method{
		case "GET":
			tournamentId := r.URL.Query().Get("id")

			tournament,err := GetTournamentById(h,ctx,tournamentId)

			if err != nil {
				http.Error(w,"Redis Get Failed",http.StatusInternalServerError)
			}
			
			fmt.Println(tournamentId,tournament)
			if tournament != nil {
				w.Header().Set("Content-Type","application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"status": "registered",
				})
			}

		case "POST":
			var newTournament Tournament
			err := json.NewDecoder(r.Body).Decode(&newTournament)
			if err != nil {
				http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
				return
			}
			fmt.Printf("%+v\n",newTournament)
			
			tournamentId, err := util.GenerateShortID(6)
			if err != nil {
				http.Error(w,"Error generating tournament id",http.StatusInternalServerError)
			}
			newTournament.Id = tournamentId
			fmt.Println("Tournament Id:",tournamentId)

			tournamentKey := fmt.Sprintf("tournament:%s",tournamentId)

			err = h.Redis.HSet(ctx,tournamentKey, map[string]interface{}{
				"Id":newTournament.Id,
				"BoardsPerRound":newTournament.BoardsPerRound,
				"TotalRounds":newTournament.TotalRounds,
				"Type":newTournament.Type,
				"Teams":newTournament.Teams,
			}).Err()
			if err != nil {
				http.Error(w,"Failed to store tournament",http.StatusInternalServerError)
			}

			h.WebSocketHub.expectedClientCounts[newTournament.Id] = newTournament.Teams


			h.WebSocketHub.OnClientCountChangeMap[newTournament.Id] = func(count int){
				fmt.Printf("Tournament %s: %d/%d clients connected\n", newTournament.Id, count, h.WebSocketHub.expectedClientCounts[newTournament.Id])
				if count == h.WebSocketHub.expectedClientCounts[newTournament.Id] {
					fmt.Println("All clients registered, start tournament!")
					h.WebSocketHub.Broadcast(newTournament.Id, map[string]interface{}{
						"TournamentReady": true,
					})
				}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newTournament)

		default:
			http.Error(w,"Method Not Allowed",http.StatusMethodNotAllowed)
	}
}

func (h *Handler) PairHandler(w http.ResponseWriter, r *http.Request){
	fmt.Println("Handle Pair",r.Method)
	ctx := r.Context()

	switch r.Method{
		case "GET":
			tournamentId := r.URL.Query().Get("tournamentId")
			pairId := r.URL.Query().Get("pairId")

			key := fmt.Sprintf("tournament:%s:pair:%s",tournamentId,pairId)
			data,err := h.Redis.HGetAll(ctx,key).Result()
			if err != nil {
				http.Error(w,"Unable to get Pair info",http.StatusInternalServerError)
			}
			if data != nil {
				var pair Pair
				if val, ok := data["Name1"]; ok {
					fmt.Sscanf(val, "%s", &pair.Name1)
				}
				if val, ok := data["Name2"]; ok {
					fmt.Sscanf(val, "%s", &pair.Name2)
				}

				fmt.Printf("%+v",pair)
				w.Header().Set("Content-Type","application/json")
				json.NewEncoder(w).Encode(pair)
			}


		case "POST":
			var newPair Pair
			err := json.NewDecoder(r.Body).Decode(&newPair)
			if err != nil {
				http.Error(w,"Invalid JSON payload",http.StatusBadRequest)
				return
			}

			fmt.Printf("%+v\n",newPair)

			counterKey := fmt.Sprintf("tournament:%s:pair_counter",newPair.TournamentId)
			pairCount,err := h.Redis.Incr(ctx,counterKey).Result()
			if err != nil {
				http.Error(w,"Couldn't get pair count",http.StatusInternalServerError)
			}
			tournament,err := GetTournamentById(h,ctx,newPair.TournamentId)
			if err != nil {
				h.Redis.Decr(ctx, counterKey)
				http.Error(w,"Couldn't get tournament",http.StatusInternalServerError)
				return
			}

			if int(pairCount) > tournament.Teams {
				h.Redis.Decr(ctx, counterKey)
				http.Error(w,"Tournament is full",http.StatusForbidden)
				return
			}

			fmt.Println("Got the",pairCount,"pair!")

			tableNum := (int)(pairCount + 1)/2
			dir := "NS"
			opp := fmt.Sprintf("%dEW",tableNum)
			if pairCount % 2 == 0{
				dir = "EW"
				opp = fmt.Sprintf("%dNS",tableNum)
			}

			newPair.Id = fmt.Sprintf("%d%s",tableNum,dir)

			fmt.Println("You are the following pair:",newPair.Id)

			pairKey := fmt.Sprintf("tournament:%s:pair:%s",newPair.TournamentId,newPair.Id)

			err = h.Redis.HSet(ctx, pairKey, map[string]interface{}{
				"Id": newPair.Id,
				"Name1": newPair.Name1,
				"Name2": newPair.Name2,
				"TournamentId":newPair.TournamentId,
			}).Err()

			if err != nil {
				h.Redis.Decr(ctx, counterKey)
				http.Error(w, "Failed to store pair", http.StatusInternalServerError)
            	return
			}

			currentBoard := (tableNum - 1) * tournament.BoardsPerRound + 1
			boardStateKey := fmt.Sprintf("tournament:%s:pair:%s:state",newPair.TournamentId,newPair.Id)
			err = h.Redis.HSet(ctx,boardStateKey,map[string]interface{}{
				"CurrentBoard":currentBoard,
				"CurrentOpp":opp,
				"CurrentRound":1,
			}).Err()
			if err != nil {
				http.Error(w, "Could not assign boards to pair", http.StatusInternalServerError)
			}

			w.Header().Set("Content-Type","application/json")
			json.NewEncoder(w).Encode(newPair)

		default:
			fmt.Println("Unknown request method")
	}
}

func (h *Handler) BoardHandler(w http.ResponseWriter, r *http.Request){
	ctx := r.Context()
	fmt.Println("Handle Board",r.Method)

	switch r.Method{
		case "GET":
			tournamentId := r.URL.Query().Get("tournamentId")
			pairId := r.URL.Query().Get("pair")

			fmt.Println("Tournament:",tournamentId,"Pair:",pairId)

			boardState,err := GetBoardStateByPairId(h,ctx,tournamentId,pairId)
			if err != nil {
				http.Error(w,"Unable to get current board number",http.StatusInternalServerError)
			}

			fmt.Printf("%+v",boardState)

			w.Header().Set("Content-Type","application/json")
			json.NewEncoder(w).Encode(boardState)

		case "POST":
			var newResult types.BoardResult
			err := json.NewDecoder(r.Body).Decode(&newResult)
			if err != nil {
				http.Error(w,"Invalid JSON payload",http.StatusBadRequest)
				return
			}

			vul := GetVulByBoardNumber(newResult.BoardNumber)
			score := scoring.CalculateScore(newResult.Contract,newResult.Direction,newResult.Result,vul)

			//Update score for the NS pair
			ResultKeyNS := fmt.Sprintf("tournament:%s:board:%d:pair:%s",newResult.TournamentId,newResult.BoardNumber,newResult.NSPairId)
			err = h.Redis.HSet(ctx,ResultKeyNS,map[string]interface{}{
				"BoardNumber": newResult.BoardNumber,
				"Vul": vul,
				"Contract": newResult.Contract,
				"Direction": newResult.Direction,
				"Result": newResult.Result,
				"NSPairId": newResult.NSPairId,
				"EWPairId": newResult.EWPairId,
				"TournamentId": newResult.TournamentId,
				"Score":score,
			}).Err()
			if err != nil {
				http.Error(w,"Failed to set NS pair result",http.StatusInternalServerError)
				return
			}

			//Update score for the EW pair
			/*ResultKeyEW := fmt.Sprintf("tournament:%s:board:%d:pair:%s",newResult.TournamentId,newResult.BoardNumber,newResult.EWPairId)
			err = h.Redis.HSet(ctx,ResultKeyEW,map[string]interface{}{
				"BoardNumber": newResult.BoardNumber,
				"Vul": vul,
				"Contract": newResult.Contract,
				"Direction": newResult.Direction,
				"Result": newResult.Result,
				"NSPairId": newResult.NSPairId,
				"EWPairId": newResult.EWPairId,
				"TournamentId": newResult.TournamentId,
				"Score":-score,
			}).Err()
			if err != nil {
				http.Error(w,"Failed to set NS pair result",http.StatusInternalServerError)
				return
			}*/


			newBoardStateNS,_,isOverNS := NextState(h,ctx,newResult.TournamentId,newResult.NSPairId)
			newBoardStateEW,_,isOverEW := NextState(h,ctx,newResult.TournamentId,newResult.EWPairId)

			if !isOverEW && !isOverNS {
				fmt.Printf("%+v\n",newBoardStateNS)
				fmt.Printf("%+v\n",newBoardStateEW)
			}
			
			if isOverNS{
				finishKey := "tournament:%s:finished_pairs"
				h.Redis.SAdd(ctx,finishKey,newResult.NSPairId)
			}
			if isOverEW{
				finishKey := "tournament:%s:finished_pairs"
				h.Redis.SAdd(ctx,finishKey,newResult.EWPairId)
			}

			response := map[string]PairStateResponse{
				"NS":{
					PairId:     newResult.NSPairId,
					BoardState: newBoardStateNS,
					IsOver:     isOverNS,
				},
				"EW":{
					PairId:     newResult.EWPairId,
					BoardState: newBoardStateEW,
					IsOver:     isOverEW,
				},
			}

			w.Header().Set("Content-Type","application/json")
			json.NewEncoder(w).Encode(response)

		default:
			fmt.Println("Unknown request method")
	}
}

func (h *Handler) PairResultsHandler(w http.ResponseWriter,r *http.Request){
	ctx := r.Context()
	fmt.Println("Handle PairResults",r.Method)
	switch r.Method{
		case "GET":
			tournamentId := r.URL.Query().Get("tournamentId")
			pairId := r.URL.Query().Get("pair")

			if tournamentId == "" || pairId == "" {
				http.Error(w, "Missing tournamentId or pair query parameter", http.StatusBadRequest)
				return
			}

			key := fmt.Sprintf("tournament:%s:pair:%s:boardResults",tournamentId,pairId)
			response,err := h.Redis.HGetAll(ctx,key).Result()
			if err != nil {
				http.Error(w,"Failed to retrieve board results",http.StatusInternalServerError)
			}

			var pairResults []PairResultByBoard
			for field,value := range response{
				var pairResult PairResultByBoard
				if err := json.Unmarshal([]byte(value),&pairResult); err != nil{
					fmt.Printf("Failed to unmarshal board %s: %v\n", field, err)
					continue 
				}
				pairResults = append(pairResults,pairResult)
			}

			w.Header().Set("Content-Type","application/json")
			json.NewEncoder(w).Encode(pairResults)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}