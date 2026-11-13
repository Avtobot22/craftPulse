package entity

import (
	"encoding/hex"
	"errors"
)

var errInvalidID = errors.New("entity: ID must be a UUID in the 8-4-4-4-12 form")

// ID holds a UUID without tying entities to a UUID library.
type ID [16]byte

func (id ID) IsZero() bool { return id == ID{} }

func (id ID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], id[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], id[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], id[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], id[8:10])
	b[23] = '-'
	hex.Encode(b[24:], id[10:])
	return string(b[:])
}

func (id ID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }

func (id *ID) UnmarshalText(b []byte) error {
	if len(b) != 36 || b[8] != '-' || b[13] != '-' || b[18] != '-' || b[23] != '-' {
		return errInvalidID
	}
	var digits [32]byte
	n := copy(digits[:], b[:8])
	n += copy(digits[n:], b[9:13])
	n += copy(digits[n:], b[14:18])
	n += copy(digits[n:], b[19:23])
	copy(digits[n:], b[24:])
	var out ID
	if _, err := hex.Decode(out[:], digits[:]); err != nil {
		return errInvalidID
	}
	*id = out
	return nil
}
