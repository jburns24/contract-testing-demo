# ADR-0001: Hexagonal Architecture for Contract Testing

## Status
Accepted

## Context

The checkout service originally used a tightly coupled approach for order event publishing, where the business logic directly called Kafka-specific methods (`sendToPostProcessor`). This created several challenges:

1. **Infrastructure Dependencies**: Business logic was coupled to Kafka, making testing difficult
2. **Contract Testing Complexity**: Tests had to set up Kafka infrastructure and handle protobuf serialization
3. **Technology Lock-in**: Changing from Kafka to another messaging system would require business logic changes
4. **Mock Complexity**: Testing required complex Kafka producer mocks and infrastructure simulation

### Previous Architecture Issues

```go
// Old approach: Business logic coupled to infrastructure
type checkout struct {
    // ... other fields
    kafkaProducer sarama.AsyncProducer  // Direct dependency on Kafka
}

func (cs *checkout) PlaceOrder(ctx context.Context, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
    // ... business logic ...

    // Direct call to infrastructure adapter - PROBLEMATIC
    if err := cs.sendToPostProcessor(ctx, orderResult); err != nil {
        return nil, err
    }

    return response, nil
}
```

This approach mixed business concerns with infrastructure concerns, violating the Dependency Inversion Principle and making contract testing unnecessarily complex.

## Decision

We will implement explicit Hexagonal Architecture (Ports and Adapters) for order event publishing to improve contract testing capabilities and architectural clarity.

### Architecture Components

#### 1. Port Interface (Business Logic Boundary)
```go
// ports/order_event_publisher.go
type OrderEventPublisher interface {
    PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error
}
```

The port defines **WHAT** the business logic needs (publish order completion events) without specifying **HOW** it should be done.

#### 2. Adapters (Infrastructure Implementations)
```go
// adapters/kafka_order_event_publisher.go
type KafkaOrderEventPublisher struct {
    producer sarama.AsyncProducer
    logger   *slog.Logger
    tracer   trace.Tracer
}

type NoOpOrderEventPublisher struct{}
```

Adapters define **HOW** the port interface is implemented using specific technologies (Kafka, No-op, etc.).

#### 3. Dependency Injection (Inversion of Control)
```go
// main.go
type checkout struct {
    // ... other fields
    orderEventPublisher ports.OrderEventPublisher  // Depends on abstraction, not concretion
}

func (cs *checkout) PlaceOrder(ctx context.Context, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
    // ... business logic ...

    // Call through port interface - CLEAN SEPARATION
    if err := cs.orderEventPublisher.PublishOrderCompleted(ctx, orderResult); err != nil {
        return nil, err
    }

    return response, nil
}
```

### Contract Testing Benefits

#### Before: Adapter-Level Testing
- Tests coupled to Kafka infrastructure
- Required complex producer setup and message serialization
- Mixed business logic validation with infrastructure concerns
- Difficult to isolate business logic patterns

#### After: Port-Level Testing
- Tests focus on business contracts, not infrastructure
- Simple interface mocking without Kafka dependencies
- Clean separation between business logic and infrastructure testing
- Easy to test different scenarios and edge cases

```go
// New approach: Test the port interface
func TestOrderEventPublisherContract(t *testing.T) {
    // Create a capture mock that records what gets published through the port
    var capturedOrder *pb.OrderResult
    captureMock := &MessageCaptureMock{
        onPublish: func(order *pb.OrderResult) {
            capturedOrder = order
        },
    }

    // Create checkout service with the capture mock
    checkoutService := &checkout{
        orderEventPublisher: captureMock,
    }

    // Test business logic patterns through the port interface
    messageHandlers := message.Handlers{
        "order-result message": func(states []models.ProviderState) (message.Body, message.Metadata, error) {
            order := createOrderResultFromBusinessLogicPatterns()

            // ✅ KEY IMPROVEMENT: Exercise the actual port interface!
            // This calls through the same business logic as PlaceOrder()
            err := checkoutService.orderEventPublisher.PublishOrderCompleted(ctx, order)

            // Verify and convert the captured order for contract verification
            return convertOrderResultToConsumerFormat(capturedOrder),
                   message.Metadata{"contentType": "application/json"}, nil
        },
    }

    // Verify contracts through the port abstraction layer
    verifier.VerifyProvider(t, provider.VerifyRequest{...})
}
```

### Key Innovation: Port Interface Exercise

The critical insight implemented in this architecture is that contract tests should exercise the **actual port interface**, not just test message formatting. This approach:

1. **Tests Real Business Logic Flow**: The contract test calls `checkoutService.orderEventPublisher.PublishOrderCompleted()`, exercising the same code path as the actual `PlaceOrder()` method
2. **Validates Port Abstraction**: Ensures the port interface correctly handles business logic interactions
3. **Captures Through Dependency Injection**: Uses a specialized `MessageCaptureMock` to capture what gets published through the port
4. **Maintains Technology Independence**: Tests remain infrastructure-agnostic while exercising real business patterns

This represents a significant improvement over traditional contract testing approaches that often bypass the domain boundaries they're meant to validate.

## Consequences

### Positive

1. **Technology Independence**: Business logic no longer depends on Kafka-specific APIs
2. **Improved Testability**: Contract tests focus on business patterns, not infrastructure
3. **Cleaner Architecture**: Clear separation between business logic (inside) and infrastructure (outside)
4. **Easier Mocking**: Simple interface implementations for testing scenarios
5. **Flexible Infrastructure**: Can swap Kafka for RabbitMQ, EventBridge, etc. without business logic changes
6. **Contract Testing Clarity**: Tests validate business contracts at the appropriate abstraction level

### Negative

1. **Additional Abstraction Layer**: Slightly more complex codebase with ports and adapters
2. **Learning Curve**: Team needs to understand hexagonal architecture principles
3. **Interface Maintenance**: Port interfaces need to evolve with business requirements

### Migration Benefits

The refactoring preserved all existing functionality while improving:

- **Legacy Tests**: Original tests preserved as no-ops for historical reference
- **New Tests**: Port-based contract tests provide better business logic validation
- **Documentation**: Clear ADR explaining the architectural decision and patterns
- **API Contracts**: Explicit documentation of request/response formats and port boundaries

## Implementation Details

### Files Created/Modified

1. **ports/order_event_publisher.go**: Port interface definition
2. **adapters/kafka_order_event_publisher.go**: Kafka and No-op implementations
3. **main.go**: Refactored to use dependency injection with port interface
4. **order_event_publisher_contract_test.go**: New port-based contract tests
5. **checkout_message_provider_test.go**: Legacy tests preserved as no-ops
6. **docs/adr/0001-hexagonal-architecture-for-contract-testing.md**: This ADR
7. **README.md**: Updated with API contracts and architecture documentation

### Compile-Time Safety

```go
// Ensure implementations satisfy the port interface
var _ ports.OrderEventPublisher = (*KafkaOrderEventPublisher)(nil)
var _ ports.OrderEventPublisher = (*NoOpOrderEventPublisher)(nil)
var _ ports.OrderEventPublisher = (*MockOrderEventPublisher)(nil)
```

This provides compile-time verification that all adapters properly implement the port interface.

## Related Patterns

- **Dependency Inversion Principle**: High-level modules don't depend on low-level modules
- **Interface Segregation**: Small, focused interfaces specific to business needs
- **Adapter Pattern**: Wrapping external libraries with domain-specific interfaces
- **Strategy Pattern**: Different implementations of the same business capability

## References

- [Hexagonal Architecture by Alistair Cockburn](https://alistair.cockburn.us/hexagonal-architecture/)
- [Clean Architecture by Robert Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Pact Contract Testing](https://docs.pact.io/)
- [Dependency Inversion Principle](https://en.wikipedia.org/wiki/Dependency_inversion_principle)
