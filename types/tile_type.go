package types

type TileType int

const (
	TileTypeGround TileType = iota + 1
	TileTypeWater
	TileTypeBuilding
)
