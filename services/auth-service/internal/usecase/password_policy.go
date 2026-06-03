package usecase

import (
	"fmt"
	"unicode"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
)

// validateAccountPassword enforces the account password policy applied at
// registration and password reset. It mirrors the client-side checks in
// frontend/src/lib/password.ts so the API cannot be bypassed with a weaker
// password than the UI allows: at least minAccountPasswordLen characters, with
// at least one letter and at least one digit. Letters are matched Unicode-aware
// so Cyrillic passwords count.
func validateAccountPassword(password string) error {
	if len(password) < minAccountPasswordLen {
		return pkgerr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minAccountPasswordLen))
	}

	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
		if hasLetter && hasDigit {
			break
		}
	}

	if !hasLetter || !hasDigit {
		return pkgerr.InvalidArgument("password must contain at least one letter and one digit")
	}

	return nil
}
