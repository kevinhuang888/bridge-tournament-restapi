package types

type MatchpointScore struct {
	PairID    string
	Direction string
	MPScore   float64
	RawScore  int
	Percentage float64
	Contract string
	ContractDirection string
	Result string
}

type BoardResult struct {
	BoardNumber  int
	Vul          string
	Contract     string
	Direction    string
	Result       string
	NSPairId     string
	EWPairId     string
	TournamentId string
	Score        int 
}