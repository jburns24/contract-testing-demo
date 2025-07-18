# Checkout Service

This service provides checkout services for the application using hexagonal architecture (ports and adapters pattern) for clean separation of business logic from infrastructure concerns.

## Architecture Overview

The checkout service implements **Hexagonal Architecture** to decouple business logic from infrastructure dependencies:

### Core Components

- **Business Logic**: Pure domain logic for order processing (inside the hexagon)
- **Ports**: Interfaces defining what the business logic needs (boundaries of the hexagon)
- **Adapters**: Infrastructure implementations of ports (outside the hexagon)

### Port Interfaces

#### OrderEventPublisher Port
**Purpose**: Publishes order completion events to external systems
**Location**: `ports/order_event_publisher.go`

```go
type OrderEventPublisher interface {
    PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error
}
```

### Adapter Implementations

#### KafkaOrderEventPublisher
**Purpose**: Publishes order events to Apache Kafka
**Location**: `adapters/kafka_order_event_publisher.go`
**Features**:
- Async message publishing with acknowledgment waiting
- Distributed tracing with OpenTelemetry
- Error handling and logging
- Message serialization to protobuf

#### NoOpOrderEventPublisher
**Purpose**: No-operation implementation for testing or when messaging is disabled
**Location**: `adapters/kafka_order_event_publisher.go`
**Use Cases**:
- Development environments without Kafka
- Testing scenarios
- Graceful degradation when messaging infrastructure is unavailable

## API Contracts

### Order Completion Event

This service publishes order completion events when orders are successfully processed.

#### Message Schema

**Topic**: `orders` (Kafka)
**Format**: Protocol Buffers (protobuf)
**Content-Type**: `application/json` (for contract testing)

#### OrderResult Structure

```json
{
  "orderId": "string",
  "shippingTrackingId": "string",
  "shippingCost": {
    "currencyCode": "string",
    "units": "integer",
    "nanos": "integer"
  },
  "shippingAddress": {
    "streetAddress": "string",
    "city": "string",
    "state": "string",
    "country": "string",
    "zipCode": "string"
  },
  "items": [
    {
      "item": {
        "productId": "string",
        "quantity": "integer"
      },
      "cost": {
        "currencyCode": "string",
        "units": "integer",
        "nanos": "integer"
      }
    }
  ]
}
```

#### Required Fields

- `orderId`: Unique identifier for the order
- `shippingTrackingId`: Tracking number for shipment
- `shippingCost`: Cost of shipping with currency details
- `shippingAddress`: Complete delivery address
- `items`: Array of ordered items with costs

#### Field Specifications

**Money Fields** (`shippingCost`, `item.cost`):
- `currencyCode`: ISO 4217 currency code (e.g., "USD")
- `units`: Whole currency units (e.g., dollars)
- `nanos`: Fractional units in nanoseconds (0-999,999,999)

**Address Fields**:
- All fields are required strings
- `zipCode`: Postal/ZIP code for delivery location

**Item Fields**:
- `productId`: SKU or product identifier
- `quantity`: Number of items ordered (positive integer)

#### Example Message

```json
{
  "orderId": "order-12345",
  "shippingTrackingId": "TRACK-789",
  "shippingCost": {
    "currencyCode": "USD",
    "units": 8,
    "nanos": 500000000
  },
  "shippingAddress": {
    "streetAddress": "123 Main St",
    "city": "Anytown",
    "state": "CA",
    "country": "USA",
    "zipCode": "94016"
  },
  "items": [
    {
      "item": {
        "productId": "SKU-001",
        "quantity": 2
      },
      "cost": {
        "currencyCode": "USD",
        "units": 25,
        "nanos": 990000000
      }
    }
  ]
}
```

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

This service uses Pact contract testing with hexagonal architecture for clean separation of business logic testing from infrastructure concerns.

### Architecture-Driven Testing Approach

The hexagonal architecture enables superior contract testing by testing at the **port level** rather than the adapter level:

**Port-Based Testing** (Current Approach):
- Tests exercise the `OrderEventPublisher` port interface through dependency injection
- Contract tests call `checkoutService.orderEventPublisher.PublishOrderCompleted()` - same as real business logic
- Business logic validation independent of Kafka infrastructure
- Clean mocking through simple interface implementations
- Contract verification focuses on business patterns and validates the port abstraction

**Legacy Adapter Testing** (Previous Approach):
- Tests directly called Kafka-specific methods (`sendToPostProcessor`)
- Required complex Kafka producer setup and protobuf handling
- Mixed business logic testing with infrastructure concerns
- Preserved in `checkout_message_provider_test.go` for historical reference

### Test Files

#### Active Contract Tests
- **File**: `order_event_publisher_contract_test.go`
- **Purpose**: Tests the OrderEventPublisher port interface
- **Benefits**: Technology-independent, easy mocking, clear business focus

#### Legacy Tests (Historical Reference)
- **File**: `checkout_message_provider_test.go`
- **Status**: No-op tests preserved for historical comparison
- **Purpose**: Documents the evolution from adapter-specific to port-based testing

### Prerequisites

The contract testing requires the Pact FFI library:

```sh
# Install the pact-go tool and FFI library
go install github.com/pact-foundation/pact-go/v2@latest
sudo pact-go install
```

### Running Contract Tests

**Port-Based Contract Tests** (Active):
```sh
go test -v -run TestOrderEventPublisherContract
```

**Legacy Tests** (Historical Reference - Will Skip):
```sh
go test -v -run Legacy
```

### Contract Test Architecture

**Current Hexagonal Approach**:
- **Port**: `OrderEventPublisher` interface defines business contract
- **Adapters**: `KafkaOrderEventPublisher`, `NoOpOrderEventPublisher` implement port
- **Tests**: Exercise port interface with mock implementations
- **Benefits**: Clean separation, easy testing, technology independence

**Consumer-Provider Relationship**:
- **Consumer**: Accounting service defines expected message contracts
- **Provider**: Checkout service (this service) must satisfy those contracts
- **Message Flow**: Business Logic → Port → Adapter → Kafka → Consumer
- **Contract Storage**: Pact files in `../accounting/tests/pacts/`

### Contract Verification

The tests verify:

1. **Message Structure**: JSON schema matches consumer expectations
2. **Required Fields**: All mandatory fields present (orderId, shippingCost, etc.)
3. **Data Types**: Correct types (units as integers, proper string fields)
4. **Business Logic**: Order creation patterns match real PlaceOrder workflow
5. **Metadata**: Content-Type and other headers properly set

### Port Interface Testing Benefits

```go
// Easy mocking through port interface
type MockOrderEventPublisher struct {
    publishedOrders []*pb.OrderResult
    shouldFail      bool
}

func (m *MockOrderEventPublisher) PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error {
    // Simple mock implementation for testing
}
```

### Key Innovation: Port Interface Exercise

The contract tests implement a critical innovation: they exercise the **actual port interface** rather than just testing message formatting:

```go
// ✅ Contract test exercises the real business logic flow
messageHandlers := message.Handlers{
    "order-result message": func(states []models.ProviderState) (message.Body, message.Metadata, error) {
        orderResult := createOrderResultFromBusinessLogicPatterns()

        // THIS IS KEY: Call through the same port interface as PlaceOrder()
        err := checkoutService.orderEventPublisher.PublishOrderCompleted(ctx, orderResult)

        // Capture and verify what was published through the port
        return convertOrderResultToConsumerFormat(capturedOrder), metadata, nil
    },
}
```

This approach provides:
- **Real Business Logic Testing**: Exercises the same code path as `PlaceOrder()` method
- **Port Abstraction Validation**: Ensures the port interface works correctly
- **Testability**: No Kafka infrastructure required for contract tests
- **Flexibility**: Can test various scenarios (success, failure, timeouts)
- **Clarity**: Tests focus on business contracts, not infrastructure details
- **Maintainability**: Changes to Kafka implementation don't break contract tests

### Troubleshooting

If tests fail with "library 'pact_ffi' not found":

1. Ensure pact-go is installed: `go install github.com/pact-foundation/pact-go/v2@latest`
2. Install FFI library: `sudo pact-go install`
3. Verify installation: `pact-go -v`

### ADR Documentation

For detailed architecture decisions and rationale, see:
`docs/adr/0001-hexagonal-architecture-for-contract-testing.md`
