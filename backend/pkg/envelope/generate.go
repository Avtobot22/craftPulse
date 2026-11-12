package envelope

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../docs/technical-specification/envelope.yaml
//go:generate go run ./schemagen -in ../../../docs/technical-specification/envelope.yaml -out schemas.gen.json
