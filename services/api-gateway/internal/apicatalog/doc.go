// Package apicatalog defines stable HTTP API error codes and messages for api-gateway.
//
// Source of truth: errors.yaml. After editing YAML, regenerate:
//
//	go generate ./internal/apicatalog/...
//
// (run from directory services/api-gateway)
//
//go:generate go run ../../tools/apierrorgen -yaml errors.yaml -out catalog_gen.go
package apicatalog
