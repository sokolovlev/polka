// Command order is the Polka order service (spec 4).
package main

import (
	"os"

	"github.com/sokolovlev/polka/libs/go/service"

	"github.com/sokolovlev/polka/services/order/internal/transport/grpcapi"
)

func main() {
	os.Exit(service.Main("order", grpcapi.Register))
}
