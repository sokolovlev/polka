// Command auth is the Polka auth service (spec 4).
package main

import (
	"os"

	"github.com/sokolovlev/polka/libs/go/service"

	"github.com/sokolovlev/polka/services/auth/internal/transport/grpcapi"
)

func main() {
	os.Exit(service.Main("auth", grpcapi.Register))
}
