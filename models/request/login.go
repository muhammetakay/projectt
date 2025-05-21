package request

import "errors"

type LoginRequest struct {
	Nickname string `json:"nickname"`
}

func (r *LoginRequest) Validate() error {
	if r.Nickname == "" {
		return errors.New("error.validation.nickname.required")
	}
	return nil
}
