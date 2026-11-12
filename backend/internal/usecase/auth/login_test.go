package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
)

func TestReturnPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		give string
		want string
	}{
		{give: "/teams/1?tab=flow", want: "/teams/1?tab=flow"},
		{give: "", want: "/"},
		{give: "teams", want: "/"},
		{give: "https://evil.test/", want: "/"},
		{give: "//evil.test", want: "/"},
		{give: "/\\evil.test", want: "/"},
		{give: "/%2F/evil.test", want: "/"},
		{give: "/a b", want: "/"},
		{give: "/a\x00", want: "/"},
		{give: "/" + strings.Repeat("a", 2048), want: "/"},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, auth.ReturnPath(tt.give))
		})
	}
}
