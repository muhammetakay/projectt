package migrations

import (
	"projectt/models"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Seed(db *gorm.DB) {
	// silent mode
	db.Logger = logger.Default.LogMode(logger.Silent)

	// users
	adminUser := models.User{Username: "admin", Email: "admin@jarvs.net", Password: "admin123"}
	db.FirstOrCreate(&adminUser, models.User{Email: adminUser.Email})

	// disable silent mode
	db.Logger = logger.Default.LogMode(logger.Info)
}
