package models

import (
	"projectt/types"
)

type Unit struct {
	ID             int            `json:"id" gorm:"primaryKey;autoIncrement"`
	UnitType       types.UnitType `json:"unit_type" gorm:"type:integer;not null"`
	OwnerCountryID int            `json:"owner_country_id" gorm:"type:integer;not null"`
	OwnerCountry   *Country       `json:"owner_country" gorm:"foreignKey:OwnerCountryID;references:ID"`
	Health         int            `json:"health" gorm:"type:integer;not null"`
	MaxHealth      int            `json:"max_health" gorm:"type:integer;not null"`
	CoordX         float32        `json:"coord_x" gorm:"default:0"`
	CoordY         float32        `json:"coord_y" gorm:"default:0"`
	DirX           float32        `json:"dir_x" gorm:"default:0"`
	DirY           float32        `json:"dir_y" gorm:"default:0"`
	MaxSpeed       float32        `json:"max_speed" gorm:"type:float;not null"`
	PlayerID       *uint          `json:"player_id" gorm:"type:integer;null"` // ID of the player controlling this unit
	Player         *Player        `json:"player,omitempty" gorm:"foreignKey:PlayerID;references:ID"`
}
