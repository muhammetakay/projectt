package socket

import (
	"projectt/binary"
	"projectt/models"
)

func getBinaryPlayer(m *models.Player) *binary.Player {
	return &binary.Player{
		ID:        uint32(m.ID),
		Nickname:  m.Nickname,
		CountryID: uint8(m.CountryID),
		EXP:       uint32(m.EXP),
		Rank:      byte(m.Rank),
		Health:    uint32(m.Health),
		MaxHealth: uint32(m.MaxHealth),
		CoordX:    float32(m.CoordX),
		CoordY:    float32(m.CoordY),
		DirX:      float32(m.DirX),
		DirY:      float32(m.DirY),
		UnitID:    m.UnitID,
	}
}

func getBinaryCountry(m models.Country) binary.Country {
	return binary.Country{
		ID:             m.ID,
		Code:           m.Code,
		IsAIControlled: m.IsAIControlled,
	}
}
