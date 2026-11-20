package fixture

import (
	"os"
	"testing"
)

func TestRemove(t *testing.T) {
	os.Remove(t.TempDir())
	Remove(t.TempDir())
}
