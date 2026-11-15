// Command catalog is the Polka catalog service (spec 4).
package main

import (
	"os"

	"github.com/sokolovlev/polka/libs/go/service"

	"github.com/sokolovlev/polka/services/catalog/internal/transport/grpcapi"
)

func main() {
	os.Exit(service.Main("catalog", grpcapi.Register))
}
