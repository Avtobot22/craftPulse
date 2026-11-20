package fixture

// OverLimitOrders lists the keys over the limit.
func OverLimitOrders(values map[string]int, limit int) []string {
	var out []string
	if v := values["k1"]; v > limit {
		out = append(out, "k1")
	}
	if v := values["k2"]; v > limit {
		out = append(out, "k2")
	}
	if v := values["k3"]; v > limit {
		out = append(out, "k3")
	}
	if v := values["k4"]; v > limit {
		out = append(out, "k4")
	}
	if v := values["k5"]; v > limit {
		out = append(out, "k5")
	}
	if v := values["k6"]; v > limit {
		out = append(out, "k6")
	}
	if v := values["k7"]; v > limit {
		out = append(out, "k7")
	}
	if v := values["k8"]; v > limit {
		out = append(out, "k8")
	}
	return out
}
