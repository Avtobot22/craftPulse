package openapi

import "errors"

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../../../docs/technical-specification/openapi.yaml
//go:generate go run ./stubgen -in openapi.gen.go -out unimplemented.gen.go
//go:generate go run ./opgen -in ../../../../../../docs/technical-specification/openapi.yaml -out operations.gen.go

var ErrNotImplemented = errors.New("operation is not implemented")

// Operation is a contract operation: Action is the x-craftpulse-action that access checks (TZ 13.4).
type Operation struct {
	ID     string
	Action string
}

const ActionPublic = "public"
