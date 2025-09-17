package rest

import (
	"encoding/json"
)

func mapToJSON(m interface{}) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
