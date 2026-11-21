package adaptersdk

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// DecodeJSONArray reads and closes a source array without retaining previously delivered items.
func DecodeJSONArray[T any](resp *http.Response, visit func(T) error) error {
	defer func() { _ = resp.Body.Close() }()
	dec := json.NewDecoder(resp.Body)
	start, err := dec.Token()
	if err != nil {
		return decodeError(err)
	}
	if start != json.Delim('[') {
		return NewError(AdapterErrorCodeInvalidResponse, "source response is not a JSON array")
	}
	for dec.More() {
		var item T
		if err := dec.Decode(&item); err != nil {
			return decodeError(err)
		}
		if err := visit(item); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return decodeError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return decodeError(err)
		}
		return NewError(AdapterErrorCodeInvalidResponse, "source response has data after the JSON array")
	}
	return nil
}
