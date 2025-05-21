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
	CoordX         int            `json:"coord_x" gorm:"default:0"`
	CoordY         int            `json:"coord_y" gorm:"default:0"`
}
