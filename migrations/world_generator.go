package migrations

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"projectt/models"
	"projectt/types"
	"sort"
	"strings"

	"gorm.io/gorm"
)

const (
	TilemapWidth  = 8192
	TilemapHeight = 4096
)

type WorldGenerator struct{}

type Vector2 struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func (m *WorldGenerator) GenerateWorld(db *gorm.DB) {
	// If the world is already generated, return
	if err := db.First(&models.Country{}).Error; err != nil {
		filePath := "migrations/countries.geojson"
		file, err := os.Open(filePath)
		if err != nil {
			fmt.Println("File not opened:", err)
			return
		}
		defer file.Close()

		// Read the file content
		content, err := io.ReadAll(file)
		if err != nil {
			fmt.Println("Error reading file:", err)
			return
		}

		// Decode JSON into a map
		var geoData map[string]interface{}
		if err := json.Unmarshal(content, &geoData); err != nil {
			fmt.Println("Error decoding JSON:", err)
			return
		}

		// Access features array
		features, ok := geoData["features"].([]interface{})
		if !ok {
			fmt.Println("Invalid GeoJSON format: features not found")
			return
		}

		var translations = make(map[string]map[string]string)

		// Begin transaction
		tx := db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				fmt.Println("Transaction rolled back due to panic:", r)
			} else {
				tx.Commit()
			}
		}()

		// Process each feature
		for i, feature := range features {
			featureMap, ok := feature.(map[string]interface{})
			if !ok {
				continue
			}

			properties, ok := featureMap["properties"].(map[string]interface{})
			if !ok {
				continue
			}

			// Print current country name and progress
			countryName := properties["NAME"].(string)
			countryCode := properties["ISO_A2"].(string)
			fmt.Printf("Processing %s... (%d/%d)\n", countryName, i+1, len(features))

			if countryName == "N. Cyprus" {
				countryCode = "NCY"
			}

			if countryCode == "-99" || len(countryCode) > 3 {
				countryCode = properties["ISO_A2_EH"].(string)
				if countryCode == "-99" {
					fmt.Printf("Skipping country %s with code %s\n", countryName, countryCode)
					continue
				}
			}

			// Extract country data
			country := models.Country{
				Name: countryName,
				Code: countryCode,
			}

			// Desired languages to translate
			languages := []string{"en", "zh", "ru", "es", "pt", "de", "ja", "fr", "pl", "zht", "ko", "tr"}

			for _, lang := range languages {
				if translations[lang] == nil {
					translations[lang] = make(map[string]string)
				}
				// Check if the translation exists
				if translation, ok := properties["NAME_"+strings.ToUpper(lang)].(string); ok {
					// Add translation to the map
					translations[lang][country.Code] = translation
				} else {
					// If not, use the English name as a fallback
					translations[lang][country.Code] = country.Name
				}
			}

			// Save to database
			if err := tx.Create(&country).Error; err != nil {
				fmt.Printf("Error creating country %s: %v\n", country.Name, err)
			}

			// geometry
			geometry, ok := featureMap["geometry"].(map[string]interface{})
			if !ok {
				continue
			}

			coordinates, ok := geometry["coordinates"].([]interface{})
			if !ok {
				continue
			}

			mapTiles := make([]models.MapTile, 0)
			if geometry["type"] == "MultiPolygon" {
				for _, polygonGroup := range coordinates {
					if pg, ok := polygonGroup.([]interface{}); ok {
						for _, polygon := range pg {
							drawPolygon(polygon, country, &mapTiles)
						}
					}
				}
			} else if geometry["type"] == "Polygon" {
				drawPolygon(coordinates[0], country, &mapTiles)
			}

			// Save map tiles to the database
			if err := tx.CreateInBatches(&mapTiles, 1000).Error; err != nil {
				log.Fatalf("Error creating map tiles for country %s: %v\n", country.Name, err)
			}
		}

		// Save translations to file - disabled for not needed anymore
		// translationsJSON, err := json.MarshalIndent(translations, "", "  ")
		// if err != nil {
		// 	fmt.Println("Error marshaling translations:", err)
		// 	return
		// }

		// if err := os.WriteFile("migrations/countries.json", translationsJSON, 0644); err != nil {
		// 	fmt.Println("Error writing translations file:", err)
		// 	return
		// }
	}
}

func drawPolygon(polygon any, country models.Country, mapTiles *[]models.MapTile) {
	if points, ok := polygon.([]interface{}); ok {
		tileCoords := make([]Vector2, 0)
		for _, point := range points {
			if coords, ok := point.([]interface{}); ok {
				if len(coords) == 2 {
					lat := coords[0].(float64)
					lon := coords[1].(float64)
					tileCoord := latLongToTileCoords(lat, lon)
					tileCoords = append(tileCoords, tileCoord)
				}
			}
		}

		// Draw polygon lines
		for i := 0; i < len(tileCoords); i++ {
			current := tileCoords[i]
			next := tileCoords[(i+1)%len(tileCoords)] // Wrap around to the first point

			drawLine(current, next, country, mapTiles)
		}

		// Fill the polygon
		fillPolygon(tileCoords, country, mapTiles)
	}
}

func drawLine(start, end Vector2, country models.Country, mapTiles *[]models.MapTile) {
	x0 := start.X
	y0 := start.Y
	x1 := end.X
	y1 := end.Y

	Abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}

	dx := Abs(x1 - x0)
	dy := Abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	for {
		// Create a new tile at the current coordinates
		mapTile := models.MapTile{
			CoordX:         x0,
			CoordY:         y0,
			OwnerCountryID: country.ID,
			TileType:       types.TileTypeGround,
		}

		// Doğrudan slice'a ekleme
		*mapTiles = append(*mapTiles, mapTile)

		if x0 == x1 && y0 == y1 {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func fillPolygon(vertices []Vector2, country models.Country, mapTiles *[]models.MapTile) {
	if len(vertices) < 3 {
		return
	}

	// Poligonun Y sınırlarını bul
	minY := math.MaxInt
	maxY := math.MinInt
	for _, v := range vertices {
		if v.Y < minY {
			minY = v.Y
		}
		if v.Y > maxY {
			maxY = v.Y
		}
	}

	// Scan-line algoritması
	for y := minY; y <= maxY; y++ {
		var intersections []int

		for i := 0; i < len(vertices); i++ {
			current := vertices[i]
			next := vertices[(i+1)%len(vertices)]

			// Yatay kenarları atla
			if current.Y == next.Y {
				continue
			}

			// Tarama çizgisi bu kenarı kesiyor mu?
			if (current.Y <= y && next.Y > y) || (next.Y <= y && current.Y > y) {
				// Kesişim X koordinatını hesapla
				x := float64(current.X) + float64(y-current.Y)/float64(next.Y-current.Y)*float64(next.X-current.X)
				intersections = append(intersections, int(math.Round(x)))
			}
		}

		// Kesişimleri sırala
		sort.Ints(intersections)

		// Çift çift al ve aralarını doldur
		for i := 0; i < len(intersections); i += 2 {
			if i+1 >= len(intersections) {
				break
			}
			startX := intersections[i]
			endX := intersections[i+1]

			for x := startX; x <= endX; x++ {
				mapTile := models.MapTile{
					CoordX:         x,
					CoordY:         y,
					OwnerCountryID: country.ID,
					TileType:       types.TileTypeGround,
				}

				// Doğrudan slice'a ekleme
				*mapTiles = append(*mapTiles, mapTile)
			}
		}
	}
}

func latLongToTileCoords(latitude, longitude float64) Vector2 {
	// Latitude'u -90 ile 90 arasına, longitude'u -180 ile 180 arasına sınırla
	latitude = math.Max(-90, math.Min(90, latitude))
	longitude = math.Max(-180, math.Min(180, longitude))

	// Longitude: -180 -> 0, 180 -> TilemapWidth
	// Latitude: -90 -> 0, 90 -> TilemapHeight (Y ekseni tersine çevrildi)
	pixelX := int(math.Round(((longitude + 180) / 360) * TilemapWidth))
	pixelY := int(math.Round(((latitude + 90) / 180) * TilemapHeight))

	// Sınırları kontrol et
	if pixelX < 0 {
		pixelX = 0
	} else if pixelX >= TilemapWidth {
		pixelX = TilemapWidth - 1
	}

	if pixelY < 0 {
		pixelY = 0
	} else if pixelY >= TilemapHeight {
		pixelY = TilemapHeight - 1
	}

	return Vector2{X: pixelX, Y: pixelY}
}
