package migrations

import (
	"log"
	"projectt/models"

	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) {
	// drop tables
	// Wipe(db)
	// create tables
	err := db.AutoMigrate(
		&models.Country{}, &models.MapTile{},
		&models.Player{}, &models.Unit{},
	)
	if err != nil {
		log.Fatal("Migration failed:", err)
	}
	// seed the database
	Seed(db)
}

func Wipe(db *gorm.DB) {
	db.Exec(`DO $$ 
DECLARE 
	r RECORD;
BEGIN 
	FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') 
	LOOP 
		EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE'; 
	END LOOP; 
END $$;`)
}
