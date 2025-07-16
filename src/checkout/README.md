# Checkout Service

This service provides checkout services for the application.

## Local Build

To build the service binary, run:

```sh
go build -o /go/bin/checkout/
```

## Docker Build

From the root directory, run:

```sh
docker compose build checkout
```

## Regenerate protos

To build the protos, run from the root directory:

```sh
make docker-generate-protobuf
```

## Bump dependencies

To bump all dependencies run:

```sh
go get -u -t ./...
go mod tidy
```

## Contract Testing

This service includes Pact contract tests to verify message compatibility with consuming services.

### Prerequisites

The contract testing requires the Pact FFI library to be installed. Install it once with:

```sh
# Install the pact-go tool and FFI library
go install github.com/pact-foundation/pact-go/v2@latest
sudo pact-go install
```

The FFI library is a native library that enables Pact verification. If you encounter linking errors mentioning `libpact_ffi`, ensure this step is completed.

### Running Contract Tests

**Provider Tests** (verify this service satisfies consumer contracts):
```sh
go test -v checkout_message_provider_test.go
```

This test verifies that the checkout service produces order-result messages that match the contracts defined by the accounting service consumer.

**Message Generation Tests** (verify message format):
```sh
go test -v -run TestOrderResultMessageGeneration checkout_message_provider_test.go
```

### Contract Test Architecture

- **Consumer**: Accounting service defines what messages it expects to receive
- **Provider**: Checkout service (this service) must satisfy those message contracts
- **Message Flow**: Checkout → Kafka → Accounting
- **Contract Storage**: Pact files generated in `../accounting/tests/pacts/`

The contract tests ensure:
1. Message structure matches expected JSON schema
2. Required fields are present (orderId, shippingCost, etc.)
3. Data types are correct (units as integers, nanos included)
4. Content-Type metadata is properly set

### Troubleshooting

If tests fail with "library 'pact_ffi' not found":
1. Ensure pact-go is installed: `go install github.com/pact-foundation/pact-go/v2@latest`
2. Install FFI library: `sudo pact-go install`
3. Verify installation: `pact-go -v`
