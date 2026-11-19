package normalize

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskKeys(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		text string
		want []string
	}{
		{"feature/PA-7-cache", []string{"PA-7"}},
		{"Fix PA-7 and XY_2-99", []string{"PA-7", "XY_2-99"}},
		{"xPA-7 PA-7suffix PA-70x", nil},
		{"PA-7 PA-7", []string{"PA-7"}},
		{"Без ключа", nil},
	} {
		assert.Equal(t, tt.want, taskKeys(tt.text), tt.text)
	}
}
