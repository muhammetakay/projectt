package request

import "errors"

type LoginRequest struct {
	Nickname string `json:"nickname"`
}

func (r *LoginRequest) Validate() error {
	if r.Nickname == "" {
		return errors.New("error.validation.nickname.required")
	}
	if len(r.Nickname) < 3 {
		return errors.New("error.validation.nickname.minlength")
	}
	return nil
}
