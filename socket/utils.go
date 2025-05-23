package socket

import (
	"projectt/binary"
	"projectt/models"
)

func getBinaryPlayer(m *models.Player) *binary.Player {
	return &binary.Player{
		ID:        uint32(m.ID),
		Nickname:  m.Nickname,
		CountryID: uint16(m.CountryID),
		EXP:       uint32(m.EXP),
		Rank:      byte(m.Rank),
		Health:    uint32(m.Health),
		MaxHealth: uint32(m.MaxHealth),
		CoordX:    uint16(m.CoordX),
		CoordY:    uint16(m.CoordY),
		UnitID:    m.UnitID,
	}
}

func getBinaryCountry(m models.Country) binary.Country {
	return binary.Country{
		ID:             m.ID,
		Name:           m.Name,
		Code:           m.Code,
		IsAIControlled: m.IsAIControlled,
	}
}
