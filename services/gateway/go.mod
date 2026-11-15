module github.com/sokolovlev/polka/services/gateway

go 1.27

require (
	github.com/go-chi/chi/v5 v5.5.0
	github.com/sokolovlev/polka/libs/go v0.0.0
	github.com/stretchr/testify v1.12.1
	go.yaml.in/yaml/v3 v3.0.6
)

require (
	github.com/caarlos0/env/v11 v11.4.2 // indirect
	golang.org/x/sync v0.25.0 // indirect
)

replace github.com/sokolovlev/polka/libs/go => ../../libs/go
