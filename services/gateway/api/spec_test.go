package api_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/sokolovlev/polka/services/gateway/api"
)

type operation struct {
	OperationID string            `yaml:"operationId"`
	Access      string            `yaml:"x-access"`
	Security    *[]map[string]any `yaml:"security"`
	Responses   map[string]any    `yaml:"responses"`
}

type document struct {
	OpenAPI string                          `yaml:"openapi"`
	Paths   map[string]map[string]yaml.Node `yaml:"paths"`
}

var methods = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true}

// The gateway's auth middleware will be driven by x-access, so every
// operation must declare a known level, and public ones must not require a
// bearer token.
func TestEveryOperationDeclaresAccess(t *testing.T) {
	var doc document
	require.NoError(t, yaml.Unmarshal(api.Spec, &doc))
	require.True(t, strings.HasPrefix(doc.OpenAPI, "3.1"))

	known := map[string]bool{
		"public": true, "authenticated": true, "verified": true, "owner": true,
		"seller": true, "seller-owner": true, "admin": true, "refresh-cookie": true, "hmac": true,
	}
	ids := map[string]bool{}
	count := 0
	for path, item := range doc.Paths {
		for method, node := range item {
			if !methods[method] {
				continue
			}
			var op operation
			require.NoError(t, node.Decode(&op), "%s %s", method, path)
			count++
			assert.True(t, known[op.Access], "%s %s: x-access %q", method, path, op.Access)
			assert.NotEmpty(t, op.OperationID, "%s %s", method, path)
			assert.False(t, ids[op.OperationID], "duplicate operationId %s", op.OperationID)
			ids[op.OperationID] = true
			if op.Access == "public" || op.Access == "hmac" {
				require.NotNil(t, op.Security, "%s %s: public operations must set security: []", method, path)
				assert.Empty(t, *op.Security, "%s %s", method, path)
			}
		}
	}
	// 6.5 lists 49 REST operations (+ /ws and the three probes).
	assert.Equal(t, 52, count)
}
