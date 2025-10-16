package types

type UnitType uint8

const (
	UnitTypeInfantry UnitType = iota + 1
	UnitTypeTank
	UnitTypeShip
	UnitTypeBattleShip
	UnitTypeHelicopter
	UnitTypeFighterJet
)
