package fixture

import "time"

func waitTooLittle() {
	time.Sleep(10)
}

func waitMilliseconds() {
	time.Sleep(10 * time.Millisecond)
}
