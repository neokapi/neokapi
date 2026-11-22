package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShownName(t *testing.T) {
	tests := []struct {
		name   string
		user   *User
		handle string
		want   string
	}{
		{"the name the person set", &User{Name: "Dana Lee", Email: "dana@example.test"}, "dana", "Dana Lee"},
		{"the handle while no name is set", &User{Email: "dana@example.test"}, "danal", "danal"},
		{"a blank name counts as none", &User{Name: "  ", Email: "dana@example.test"}, "danal", "danal"},
		{"the email before the @ without a handle", &User{Email: "dana@example.test"}, "", "dana"},
		{"nothing for no user", nil, "dana", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShownName(tt.user, tt.handle))
		})
	}
}
