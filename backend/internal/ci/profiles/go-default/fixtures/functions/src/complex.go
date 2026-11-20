package fixture

// StatusText has one branch per status: cyclomatic complexity above the threshold of 10.
func StatusText(code int) string {
	switch code {
	case 1:
		return "code-1"
	case 2:
		return "code-2"
	case 3:
		return "code-3"
	case 4:
		return "code-4"
	case 5:
		return "code-5"
	case 6:
		return "code-6"
	case 7:
		return "code-7"
	case 8:
		return "code-8"
	case 9:
		return "code-9"
	case 10:
		return "code-10"
	case 11:
		return "code-11"
	case 12:
		return "code-12"
	}
	return "unknown"
}
