package models

type Country struct {
	ID             uint8  `json:"id" gorm:"primaryKey;autoIncrement"`
	Name           string `json:"name" gorm:"type:varchar(255);not null;unique"`
	Code           string `json:"code" gorm:"type:varchar(3);not null;unique"`
	IsAIControlled bool   `json:"is_ai_controlled" gorm:"type:boolean;default:false"`
}
