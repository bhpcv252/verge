package openapi

import _ "embed"

// spec is the full OpenAPI 3.0 YAML specification, embedded at build time
//
//go:embed openapi.yaml
var Spec []byte
