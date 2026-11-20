package fixture

import "fmt"

func printWrongVerb(name string) {
	fmt.Printf("user %d\n", name)
}

func printRightVerb(name string) {
	fmt.Printf("user %s\n", name)
}
