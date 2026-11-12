package entity

const _redacted = "[REDACTED]"

type Secret string

func (s Secret) Value() string { return string(s) }

func (Secret) String() string { return _redacted }

func (Secret) GoString() string { return _redacted }

func (Secret) MarshalText() ([]byte, error) { return []byte(_redacted), nil }
