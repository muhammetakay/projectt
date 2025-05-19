package request

import "errors"

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *LoginRequest) Validate() error {
	if r.Username == "" {
		return errors.New("error.validation.username.required")
	}
	if len(r.Username) < 3 {
		return errors.New("error.validation.username.minlength")
	}
	if r.Password == "" {
		return errors.New("error.validation.password.required")
	}
	if len(r.Password) < 6 {
		return errors.New("error.validation.password.minlength")
	}
	return nil
}
