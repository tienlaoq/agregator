module github.com/tienlao/agregator/services/review-service

go 1.25.5

require (
	github.com/jackc/pgx/v5 v5.7.2
	github.com/nats-io/nats.go v1.37.0
	github.com/rs/zerolog v1.33.0
	github.com/stretchr/testify v1.11.1
	github.com/tienlao/agregator/gen/go v0.0.0
	github.com/tienlao/agregator/pkg v0.0.0
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/tienlao/agregator/gen/go => ../../gen/go
	github.com/tienlao/agregator/pkg => ../../pkg
)
