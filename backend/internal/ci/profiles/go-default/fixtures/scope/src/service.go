package fixture

import "os"

// Remove deletes a file and ignores the error.
func Remove(name string) {
	os.Remove(name)
}
