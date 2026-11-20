package lib

import "os"

// Remove is vendored code with the same problem.
func Remove(name string) {
	os.Remove(name)
}
