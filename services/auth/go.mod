module github.com/sokolovlev/polka/services/auth

go 1.27

require (
	github.com/sokolovlev/polka/libs/go v0.0.0
	github.com/stretchr/testify v1.12.1
	google.golang.org/grpc v1.85.0
)

require (
	github.com/caarlos0/env/v11 v11.4.2 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261110000943-09e21b237f2c // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/sokolovlev/polka/libs/go => ../../libs/go
