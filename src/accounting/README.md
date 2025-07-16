# Accounting Service

This service consumes new orders from a Kafka topic.

## API Contract (Kafka Message)

The Accounting service expects **binary protobuf** payloads on the `orders` topic. Each message must be an `OrderResult` message defined in [`pb/demo.proto`](../../pb/demo.proto).

### `OrderResult` schema (excerpt)

```proto
message OrderResult {
  string   order_id               = 1;  // required
  string   shipping_tracking_id   = 2;  // required
  Money    shipping_cost          = 3;  // required
  Address  shipping_address       = 4;  // required
  repeated OrderItem items        = 5;  // required (must contain at least one element)
}

message OrderItem {
  CartItem item  = 1; // required
  Money    cost  = 2; // required
}

// Supporting messages
message Address {
  string street_address = 1;
  string city           = 2;
  string state          = 3;
  string country        = 4;
  string zip_code       = 5;
}

message Money {
  string currency_code = 1; // ISO-4217 code, e.g. "USD"
  int64  units         = 2; // whole units (dollars)
  int32  nanos         = 3; // fractional part in nanoseconds
}
```

### Example payload (JSON view)

```jsonc
{
  "order_id": "2",
  "shipping_tracking_id": "1234567890",
  "shipping_cost": { "currency_code": "USD", "units": 100, "nanos": 0 },
  "shipping_address": {
    "street_address": "123 Main St",
    "city": "Anytown",
    "state": "CA",
    "country": "USA",
    "zip_code": "12345"
  },
  "items": [
    {
      "item": { "product_id": "1", "quantity": 2 },
      "cost": { "currency_code": "USD", "units": 100, "nanos": 0 }
    }
  ]
}
```

The binary representation of this JSON is produced with `proto.Marshal(orderResult)` in the **checkout** service and deserialised with `OrderResult.Parser.ParseFrom(bytes)` in the **accounting** service.

If any required field is missing, the consumer logs an error and the message is skipped.

## Local Build

To build the service binary, run:

```sh
mkdir -p src/accounting/proto/ # root context
cp pb/demo.proto src/accounting/proto/demo.proto # root context
dotnet build # accounting service context
```

## Docker Build

From the root directory, run:

```sh
docker compose build accounting
```

## Bump dependencies

To bump all dependencies run in Package manager:

```sh
Update-Package -ProjectName Accounting
