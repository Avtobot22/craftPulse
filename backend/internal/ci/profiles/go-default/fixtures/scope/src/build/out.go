package build

import "os"

// Remove is build output with the same problem.
func Remove(name string) {
	os.Remove(name)
}
