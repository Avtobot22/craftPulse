package fixture

import "encoding/json"

func decodeUnchecked(data []byte) map[string]int {
	var v map[string]int
	json.Unmarshal(data, &v)
	return v
}

func decodeChecked(data []byte) (map[string]int, error) {
	var v map[string]int
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}
