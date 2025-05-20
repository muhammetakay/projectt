package types

type UnitType int

const (
	UnitTypeTank UnitType = iota + 1
	UnitTypeShip
	UnitTypeBattleShip
	UnitTypeHelicopter
	UnitTypeFighterJet
)
