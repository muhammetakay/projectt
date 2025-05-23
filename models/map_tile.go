package models

import (
	"projectt/types"
	"time"
)

type MapTile struct {
	ID                  int            `json:"id" gorm:"primaryKey;autoIncrement"`
	CoordX              uint16         `json:"coord_x" gorm:"type:integer;not null"`
	CoordY              uint16         `json:"coord_y" gorm:"type:integer;not null"`
	OwnerCountryID      uint8          `json:"owner_country_id" gorm:"type:integer;not null"`
	OwnerCountry        *Country       `json:"owner_country,omitempty" gorm:"foreignKey:OwnerCountryID;references:ID"`
	TileType            types.TileType `json:"tile_type" gorm:"type:integer;not null"`
	PrefabID            *uint16        `json:"prefab_id,omitempty" gorm:"null"`
	IsBorder            bool           `json:"is_border" gorm:"type:boolean;not null;default:false"`
	OccupiedByCountryID *uint8         `json:"occupied_by_country_id,omitempty" gorm:"type:integer;null"`
	OccupiedByCountry   *Country       `json:"occupied_by_country,omitempty" gorm:"foreignKey:OccupiedByCountryID;references:ID"`
	OccupiedAt          *time.Time     `json:"occupied_at,omitempty" gorm:"type:timestamp;null"`
}
