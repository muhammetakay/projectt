package types

type UnitType int

const (
	UnitTypeInfantry UnitType = iota + 1
	UnitTypeTank
	UnitTypeShip
	UnitTypeBattleShip
	UnitTypeHelicopter
	UnitTypeFighterJet
)
