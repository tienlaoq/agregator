package usecase

import (
	"strings"
	"testing"
)

func TestValidateAccountPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		wantMsg  string // substring expected in the error
	}{
		{name: "valid letters and digit", password: "register-pass1", wantErr: false},
		{name: "valid cyrillic and digit", password: "пароль123", wantErr: false},
		{name: "too short", password: "ab1", wantErr: true, wantMsg: "at least 8 characters"},
		{name: "letters only", password: "onlyletters", wantErr: true, wantMsg: "one letter and one digit"},
		{name: "digits only", password: "12345678", wantErr: true, wantMsg: "one letter and one digit"},
		{name: "exactly 8 with letter and digit", password: "abcdefg1", wantErr: false},
		{name: "symbols with letter and digit", password: "a1!@#$%^", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccountPassword(tt.password)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.password)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for %q, got %v", tt.password, err)
			}
		})
	}
}
