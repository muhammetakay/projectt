package models

import (
	"math"
	"projectt/types"

	"gorm.io/gorm"
)

type Player struct {
	gorm.Model
	Nickname  string           `json:"nickname" gorm:"unique;not null"`
	UserID    int              `json:"user_id" gorm:"type:integer;null"` // Game user ID
	CountryID int              `json:"country_id" gorm:"not null"`
	Country   *Country         `json:"country" gorm:"foreignKey:CountryID;references:ID"`
	EXP       int              `json:"exp" gorm:"default:0"`
	Rank      types.PlayerRank `json:"rank" gorm:"type:integer;default:1"`
	Health    int              `json:"health" gorm:"default:100"`
	CoordX    int              `json:"coord_x" gorm:"default:0"`
	CoordY    int              `json:"coord_y" gorm:"default:0"`
	UnitID    *int             `json:"unit_id" gorm:"type:tinyint;null"` // ID of the unit the player is controlling
	Unit      *Unit            `json:"unit" gorm:"foreignKey:UnitID;references:ID"`
}

// Level calculates player level based on EXP with logarithmic progression
func (m *Player) Level() int {
	if m.EXP == 0 {
		return 1
	}

	// Base EXP required for level 1
	baseExp := 1000.0
	// Growth factor (higher = steeper progression)
	growthFactor := 1.5

	// Calculate level using logarithmic formula
	// level = log(EXP/baseExp) / log(growthFactor) + 1
	level := int(math.Log(float64(m.EXP)/baseExp)/math.Log(growthFactor)) + 1

	// Ensure minimum level is 1
	if level < 1 {
		return 1
	}

	return level
}

// ExpForLevel returns required EXP for a specific level
func (m *Player) ExpForLevel(level int) int {
	if level <= 1 {
		return 0
	}

	baseExp := 1000.0
	growthFactor := 1.5

	requiredExp := baseExp * math.Pow(growthFactor, float64(level-1))
	return int(requiredExp)
}
