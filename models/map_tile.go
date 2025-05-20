package models

import (
	"projectt/types"
	"time"
)

type MapTile struct {
	ID                  int            `json:"id" gorm:"primaryKey;autoIncrement"`
	CoordX              int            `json:"coord_x" gorm:"type:integer;not null"`
	CoordY              int            `json:"coord_y" gorm:"type:integer;not null"`
	OwnerCountryID      int            `json:"owner_country_id" gorm:"type:integer;not null"`
	OwnerCountry        *Country       `json:"owner_country" gorm:"foreignKey:OwnerCountryID;references:ID"`
	TileType            types.TileType `json:"tile_type" gorm:"type:integer;not null"`
	TileSprite          *string        `json:"tile_sprite" gorm:"type:varchar(255);null"`
	IsBorder            bool           `json:"is_border" gorm:"type:boolean;not null;default:false"`
	OccupiedByCountryID *int           `json:"occupied_by_country_id" gorm:"type:integer;null"`
	OccupiedByCountry   *Country       `json:"occupied_by_country" gorm:"foreignKey:OccupiedByCountryID;references:ID"`
	OccupiedAt          *time.Time     `json:"occupied_at" gorm:"type:timestamp;null"`
}
