package openapi

import "errors"

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../../../docs/technical-specification/openapi.yaml
//go:generate go run ./stubgen -in openapi.gen.go -out unimplemented.gen.go

var ErrNotImplemented = errors.New("operation is not implemented")
