package types

type PlayerRank int

const (
	PlayerRankCitizen PlayerRank = iota + 1
	PlayerRankSoldier
	PlayerRankGeneral
	PlayerRankDiplomat
	PlayerRankLeader
)
