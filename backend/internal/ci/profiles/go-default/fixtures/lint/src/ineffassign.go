package fixture

func overwritten(n int) int {
	total := n * 2
	total = n + 1
	return total
}

func accumulated(n int) int {
	total := n * 2
	total += n
	return total
}
