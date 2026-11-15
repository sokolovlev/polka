// Package api embeds the gateway's OpenAPI 3.1 description (spec 6.5), the
// contract the REST handlers implement and requests are validated against.
package api

import _ "embed"

// Spec is the OpenAPI document.
//
//go:embed openapi.yaml
var Spec []byte
