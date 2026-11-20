package fixture

func forgotten() string {
	return "never called"
}

func called() string {
	return "called"
}

// Exported keeps called in use.
func Exported() string {
	return called()
}

// Use keeps the other examples in use.
func Use() {
	_ = decodeUnchecked(nil)
	_, _ = decodeChecked(nil)
	printWrongVerb("a")
	printRightVerb("a")
	_ = overwritten(1)
	_ = accumulated(1)
	waitTooLittle()
	waitMilliseconds()
}
