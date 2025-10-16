package models

import (
	"projectt/types"
)

type Unit struct {
	ID             uint           `json:"id" gorm:"primaryKey;autoIncrement"`
	UnitType       types.UnitType `json:"unit_type" gorm:"type:smallint;not null"`
	OwnerCountryID uint8          `json:"owner_country_id" gorm:"type:not null"`
	OwnerCountry   *Country       `json:"owner_country" gorm:"foreignKey:OwnerCountryID;references:ID"`
	Health         int            `json:"health" gorm:"type:integer;not null"`
	MaxHealth      int            `json:"max_health" gorm:"type:integer;not null"`
	CoordX         float32        `json:"coord_x" gorm:"default:0"`
	CoordY         float32        `json:"coord_y" gorm:"default:0"`
	DirX           float32        `json:"dir_x" gorm:"default:0"`
	DirY           float32        `json:"dir_y" gorm:"default:0"`
	MaxSpeed       float32        `json:"max_speed" gorm:"type:float;not null"`

	// Players related fields
	MaxPassengers uint8    `json:"max_passengers" gorm:"smallint;not null"`
	ControllerID  *uint    `json:"controller_id"`
	Controller    *Player  `json:"controller" gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Passengers    []Player `json:"passengers" gorm:"foreignKey:UnitID"`
}
