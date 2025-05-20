package migrations

import (
	"projectt/models"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Seed(db *gorm.DB) {
	// silent mode
	db.Logger = logger.Default.LogMode(logger.Silent)

	// generate world
	worldGenerator := WorldGenerator{}
	worldGenerator.GenerateWorld(db)

	// get a country
	var country models.Country
	db.First(&country, "code = ?", "TR")
	// get first tile of the country
	var tile models.MapTile
	db.First(&tile, "owner_country_id = ?", country.ID)

	// player
	testPlayer := models.Player{Nickname: "Ryuzaki", CountryID: country.ID, CoordX: tile.CoordX, CoordY: tile.CoordY}
	db.FirstOrCreate(&testPlayer, models.Player{Nickname: testPlayer.Nickname})

	// disable silent mode
	db.Logger = logger.Default.LogMode(logger.Info)
}
