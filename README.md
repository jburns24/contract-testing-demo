# Pact Contract Testing Demo

> A fork of the [OpenTelemetry Demo](https://github.com/open-telemetry/opentelemetry-demo) with [Pact.io](https://pact.io) contract testing added to demonstrate how consumer-driven contracts can gate releases in CI/CD pipelines.

## What This Demo Shows

This repository demonstrates how to leverage **Pact Broker OSS** to gate releases using consumer-driven contract testing. It showcases two different contract testing patterns across polyglot microservices:

### 1. Frontend → Shipping (Synchronous REST API)

A standard HTTP/REST contract between the Frontend and Shipping services.

| Service | Tech Stack | Role |
|---------|------------|------|
| **Frontend** | TypeScript / Next.js | Consumer - Makes HTTP POST requests to `/get-quote` |
| **Shipping** | Rust / Actix-web | Provider - Exposes REST API for shipping quotes |

### 2. Accounting → Checkout (Asynchronous Message via Kafka)

An async message contract where the Checkout service publishes `OrderResult` protobuf messages to a Kafka topic that Accounting consumes.

| Service | Tech Stack | Role |
|---------|------------|------|
| **Accounting** | C# / .NET 8 | Consumer - Consumes `OrderResult` messages from Kafka |
| **Checkout** | Go | Provider - Publishes `OrderResult` messages to Kafka |

---

## How to Run

### 1. Fork this Repository

Fork this repo to your own GitHub account so you can configure secrets and run workflows.

### 2. Run a Local Pact Broker OSS Instance

Create a `docker-compose-pact-broker.yml` file with the following content:

```yaml
services:
  postgres:
    image: postgres:14
    healthcheck:
      test: psql postgres --command "select 1" -U postgres
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: password
      POSTGRES_DB: postgres

  pact-broker:
    image: pactfoundation/pact-broker:latest
    ports:
      - "9292:9292"
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      PACT_BROKER_PORT: 9292
      PACT_BROKER_DATABASE_USERNAME: postgres
      PACT_BROKER_DATABASE_PASSWORD: password
      PACT_BROKER_DATABASE_HOST: postgres
      PACT_BROKER_DATABASE_NAME: postgres
      PACT_BROKER_BASIC_AUTH_USERNAME: pact
      PACT_BROKER_BASIC_AUTH_PASSWORD: liatrio
      PACT_BROKER_LOG_LEVEL: INFO

volumes:
  postgres_data:
```

Start the broker:

```bash
docker-compose -f docker-compose-pact-broker.yml up -d
```

### 3. Expose Your Broker via ngrok

```bash
ngrok http 9292
```

This creates a public HTTPS endpoint (e.g., `https://abc123.ngrok.io`) that GitHub Actions can reach.

### 4. Add Repository Secrets

In your fork, go to **Settings → Secrets and variables → Actions** and add:

| Secret | Value |
|--------|-------|
| `PACT_BROKER_URL` | Your ngrok URL (e.g., `https://abc123.ngrok.io`) |
| `PACT_BROKER_USERNAME` | `pact` (if using the docker-compose above) |
| `PACT_BROKER_PASSWORD` | `liatrio` (if using the docker-compose above) |

### 5. Demonstrate a Breaking Change

To see the contract testing gate in action:

1. **Make a consumer change that expects new fields:**
   - Edit `src/frontend/gateways/http/Shipping.gateway.spec.ts` to add a new field expectation in the contract
   - Push the change → Consumer workflow publishes new contract to broker

2. **Observe the provider verification fail:**
   - The `shipping-provider-pact.yml` workflow will pull the new contract
   - The `can-i-deploy` check will fail because the provider doesn't satisfy the new contract
   - The deployment is **gated** until the provider is updated

3. **Fix the provider:**
   - Update the Shipping service to include the new field
   - Push the change → Provider verification passes
   - `can-i-deploy` succeeds → Deployment proceeds

---

## Consumer-Driven Contracts & Release Gating

### How Contract Testing Inverts the Relationship

Traditional API testing has providers define their API, and consumers hope it doesn't change. **Consumer-driven contract testing inverts this relationship**:

1. **Consumers define what they need** - Each consumer writes a Pact test that declares exactly which endpoints, fields, and behaviors they depend on
2. **Contracts are published to a broker** - These expectations become the "contract" stored in the Pact Broker
3. **Providers verify against consumer contracts** - Before releasing, providers must prove they satisfy all consumer expectations
4. **Breaking changes are caught before deployment** - If a provider change would break a consumer, CI fails

This means **consumers are in control**: they specify what they consume, and providers must honor those contracts. A provider cannot unilaterally remove a field or change a response format if any consumer depends on it.

### The `can-i-deploy` Gate

The `pact-cli broker can-i-deploy` command is the critical gate that prevents breaking changes from reaching production:

```bash
pact-cli broker can-i-deploy \
  --pacticipant ShippingService \
  --version ${GIT_COMMIT} \
  --to production
```

**What this command does:**

1. **Queries the Pact Broker** for all consumers that have contracts with `ShippingService`
2. **Checks verification status** - Has this provider version (Git commit) successfully verified all contracts from consumers currently tagged as `production`?
3. **Returns pass/fail** - If any contract is unverified or failed verification, the command exits with a non-zero code, blocking deployment

**How this prevents breaking changes:**

| Scenario | What Happens |
|----------|-------------|
| Provider removes a field that consumers use | Consumer contracts still expect the field → Provider verification fails → `can-i-deploy` blocks release |
| Provider changes response structure | Consumer contracts expect old structure → Verification fails → Release blocked |
| Consumer adds new field expectation | Consumer publishes new contract → Provider hasn't verified it yet → Consumer's `can-i-deploy` blocks until provider catches up |
| Compatible change (additive) | Provider adds optional field → Existing contracts still pass → Release proceeds |

### Version Tracking & Production Tagging

The provider verification tests use `PublishOptions` to record results back to the broker:

```rust
let publish_options = if let (Ok(commit), Ok(branch)) =
    (std::env::var("GIT_COMMIT"), std::env::var("GIT_BRANCH")) {
    Some(PublishOptions {
        provider_version: Some(commit),
        provider_branch: Some(branch),
        ..Default::default()
    })
} else {
    None
};
```

**Why version tracking matters:**

- **`provider_version`** (Git commit SHA) - Creates a unique identifier so the broker knows exactly which code was verified
- **`provider_branch`** - Enables branch-based workflows (e.g., verify against `main` consumers before merging)

**Production tagging** closes the loop:

```bash
pact-cli broker create-or-update-version \
  --pacticipant ShippingService \
  --version ${GIT_COMMIT} \
  --tag production
```

This tells the broker "this version is now deployed to production." Future consumer contract changes will be verified against this tagged version, ensuring consumers don't deploy contracts the production provider can't satisfy.

---

## Relevant Files

### CI/CD Workflows

All Pact contract testing CI workflows are in `.github/workflows/`:

| Workflow | Purpose |
|----------|---------|
| `frontend-consumer-pact.yml` | Runs Frontend consumer tests, publishes contracts to broker |
| `shipping-provider-pact.yml` | Verifies Shipping provider against Frontend contracts |
| `accounting-consumer-pact.yml` | Runs Accounting consumer tests, publishes contracts to broker |
| `checkout-provider-pact.yml` | Verifies Checkout provider against Accounting contracts |

### Frontend → Shipping (REST API)

| File | Description |
|------|-------------|
| `src/frontend/gateways/http/Shipping.gateway.spec.ts` | **Consumer Pact test** - Defines expected HTTP contract for `/get-quote` endpoint. Uses `@pact-foundation/pact` to mock the Shipping service and generate a contract file. |
| `src/shipping/tests/contract_verification.rs` | **Provider verification test** - Verifies the Rust Shipping service satisfies the Frontend contract. Intelligently uses broker or local pact files based on environment. Publishes verification results with Git commit/branch for `can-i-deploy` gating. |

### Accounting → Checkout (Kafka Messages)

| File | Description |
|------|-------------|
| `src/accounting/tests/ConsumerMessagePactTests.cs` | **Consumer Pact test** - Defines expected `OrderResult` message contract. Uses reflection to invoke the private `ProcessMessage` method to test the real parsing logic. See ADR below for workaround details. |
| `src/checkout/order_event_publisher_contract_test.go` | **Provider verification test** - Uses hexagonal architecture to test the `OrderEventPublisher` port interface. This ensures the business logic generates `OrderResult` objects that match consumer expectations. |
| `src/checkout/checkout_message_provider_test.go` | Legacy provider test (preserved for historical reference, now superseded by `order_event_publisher_contract_test.go`) |

### Architecture Decision Records

| File | Description |
|------|-------------|
| `src/accounting/tests/ADR-001-protobuf-message-pact.md` | Documents the **JSON façade workaround** for Protobuf message contracts. PactNet doesn't yet support the Pact Plugin system for binary codecs, so we represent protos as JSON in the contract definition, then convert JSON → Protobuf bytes inside the verification lambda. This exercises the real consumer code while working within PactNet's current limitations. |

---

## Key Testing Patterns

### Hexagonal Architecture & Port Interfaces

#### Key Terminology

| Term | Definition |
|------|------------|
| **Hexagonal Architecture** | An architectural pattern (also called "Ports and Adapters") that isolates business logic from external concerns like databases, message queues, and APIs |
| **Port** | An interface that defines how the application interacts with the outside world (e.g., `OrderEventPublisher` interface) |
| **Adapter** | A concrete implementation of a port that handles the actual infrastructure (e.g., `KafkaOrderEventPublisher` implements `OrderEventPublisher`) |
| **Business Logic** | The core domain rules that should be independent of infrastructure choices |

#### Why Test the Port Interface?

The most valuable contract tests exercise **real business logic**, not just serialization. This means:

1. **Testing at the port level** ensures the business logic generates correct data structures
2. **Mocking the adapter** (not the business logic) isolates infrastructure concerns
3. **Contract compliance is verified where it matters** - at the boundary between business logic and infrastructure

#### Provider Test Pattern (Go/Checkout)

The `order_event_publisher_contract_test.go` demonstrates this pattern:

```go
// Define the port interface (in ports/order_event_publisher.go)
type OrderEventPublisher interface {
    PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error
}

// In the contract test: create a mock that captures what the business logic produces
captureMock := &MessageCaptureMock{
    onPublish: func(order *pb.OrderResult) {
        capturedOrder = order  // Capture for Pact verification
    },
}

// Wire the mock into the service
checkoutService := &checkout{
    orderEventPublisher: captureMock,
}

// Exercise the actual port interface - this runs real business logic!
err := checkoutService.orderEventPublisher.PublishOrderCompleted(ctx, orderResult)
```

**What this achieves:**
- The `OrderResult` is created by the same business logic patterns used in production (`createOrderResultFromBusinessLogicPatterns()`)
- The port interface is exercised, validating that the business logic → message flow works correctly
- Infrastructure (Kafka) is not needed - the mock captures the message for Pact verification
- If the business logic changes how it constructs `OrderResult`, the contract test will catch incompatibilities

#### Sometimes Refactoring is Required

Not all codebases start with clean port/adapter separation. To enable effective contract testing, you may need to:

1. **Extract interfaces** - Define a port interface for your message publisher or API client
2. **Inject dependencies** - Pass the port implementation into your service rather than instantiating it directly
3. **Separate business logic from serialization** - Ensure the data structure creation happens in testable business code, not buried in adapter code

The `checkout_message_provider_test.go` (legacy) vs `order_event_publisher_contract_test.go` (current) files in this repo show this evolution - the codebase was refactored to enable port-based testing.

### Reflection-Based Consumer Testing (C#/Accounting)

Sometimes the code you need to test isn't public. The `ConsumerMessagePactTests.cs` uses reflection to invoke private methods:

```csharp
// Get the internal Consumer type
var consumerType = Type.GetType("Accounting.Consumer, Accounting")!;

// Create a real Kafka message with protobuf bytes
var kafkaMsg = Activator.CreateInstance(kafkaMsgType)!;
kafkaMsgType.GetProperty("Value")!.SetValue(kafkaMsg, protoBytes);

// Invoke private ProcessMessage using reflection
var processMethod = consumerType.GetMethod(
    "ProcessMessage",
    BindingFlags.Instance | BindingFlags.NonPublic
);
processMethod.Invoke(consumer, new[] { kafkaMsg });
```

**Why this matters:**
- The contract test exercises the **real parsing and processing logic** inside the consumer
- If `ProcessMessage` fails to parse the protobuf or throws an exception, the Pact test fails
- This validates that the consumer can actually handle messages matching the contract, not just that a mock can

**Trade-off:** Reflection-based tests are more brittle (they break if method names change), but they provide much higher confidence than testing against mocks alone.


