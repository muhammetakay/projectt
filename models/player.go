package models

import (
	"encoding/json"
	"math"
	"projectt/types"
	"time"
)

type Player struct {
	Model
	Nickname  string           `json:"nickname" gorm:"unique;not null"`
	UserID    uint             `json:"user_id" gorm:"type:integer;null"` // Game user ID
	CountryID uint8            `json:"country_id" gorm:"not null"`
	Country   *Country         `json:"country,omitempty" gorm:"foreignKey:CountryID;references:ID"`
	EXP       uint             `json:"exp" gorm:"default:0"`
	Level     uint             `json:"level" gorm:"-"`
	Rank      types.PlayerRank `json:"rank" gorm:"type:integer;default:1"`
	Health    uint             `json:"health" gorm:"default:100"`
	MaxHealth uint             `json:"max_health" gorm:"default:100"`
	CoordX    float32          `json:"coord_x" gorm:"default:0"`
	CoordY    float32          `json:"coord_y" gorm:"default:0"`
	DirX      float32          `json:"dir_x" gorm:"default:0"`
	DirY      float32          `json:"dir_y" gorm:"default:0"`
	UnitID    *uint16          `json:"unit_id" gorm:"type:tinyint;null"` // ID of the unit the player is controlling
	Unit      *Unit            `json:"unit,omitempty" gorm:"foreignKey:UnitID;references:ID"`

	// Movement fields
	LastUpdatedTicks float32   `json:"last_updated_ticks" gorm:"-"`
	LastUpdated      time.Time `json:"last_updated" gorm:"-"`
}

func (m Player) MarshalJSON() ([]byte, error) {
	type Alias Player
	m.Level = m.GetLevel()
	return json.Marshal(Alias(m))
}

func (m *Player) Copy() *Player {
	return &Player{
		Model:            m.Model,
		Nickname:         m.Nickname,
		UserID:           m.UserID,
		CountryID:        m.CountryID,
		Country:          m.Country,
		EXP:              m.EXP,
		Level:            m.Level,
		Rank:             m.Rank,
		Health:           m.Health,
		MaxHealth:        m.MaxHealth,
		CoordX:           m.CoordX,
		CoordY:           m.CoordY,
		DirX:             m.DirX,
		DirY:             m.DirY,
		UnitID:           m.UnitID,
		Unit:             m.Unit,
		LastUpdatedTicks: m.LastUpdatedTicks,
		LastUpdated:      m.LastUpdated,
	}
}

// Level calculates player level based on EXP with logarithmic progression
func (m *Player) GetLevel() uint {
	if m.EXP == 0 {
		return 1
	}

	// Base EXP required for level 1
	baseExp := 1000.0
	// Growth factor (higher = steeper progression)
	growthFactor := 1.5

	// Calculate level using logarithmic formula
	// level = log(EXP/baseExp) / log(growthFactor) + 1
	level := uint(math.Log(float64(m.EXP)/baseExp)/math.Log(growthFactor)) + 1

	// Ensure minimum level is 1
	if level < 1 {
		return 1
	}

	return level
}

// ExpForLevel returns required EXP for a specific level
func (m *Player) ExpForNextLevel(level int) int {
	if level <= 1 {
		return 0
	}

	baseExp := 1000.0
	growthFactor := 1.5

	requiredExp := baseExp * math.Pow(growthFactor, float64(level-1))
	return int(requiredExp)
}

func (m *Player) GetChunkCoord(chunkSize int) (uint16, uint16) {
	chunkX := uint16(m.CoordX) / uint16(chunkSize)
	chunkY := uint16(m.CoordY) / uint16(chunkSize)
	return chunkX, chunkY
}

func (m *Player) GetCurrentSpeed() float32 {
	if m.Unit != nil {
		return m.Unit.MaxSpeed
	}
	return 15.0 // default walking speed
}

func (m *Player) GetUnitType() types.UnitType {
	if m.Unit != nil {
		return m.Unit.UnitType
	}
	return types.UnitTypeInfantry // default unit type
}

func (m *Player) IsMoving() bool {
	return m.DirX != 0 || m.DirY != 0
}
