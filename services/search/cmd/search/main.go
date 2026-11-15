// Command search is the Polka search service (spec 4).
package main

import (
	"os"

	"github.com/sokolovlev/polka/libs/go/service"

	"github.com/sokolovlev/polka/services/search/internal/transport/grpcapi"
)

func main() {
	os.Exit(service.Main("search", grpcapi.Register))
}
