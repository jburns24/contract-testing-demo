# Accounting Service

This service consumes new orders from a Kafka topic and processes them for accounting purposes.

## Architecture Overview

The accounting service follows a **Consumer-Port-Adapter** pattern:

- **Adapter**: `Consumer.StartListening()` - Handles Kafka message consumption and connection management
- **Port**: `Consumer.ProcessMessage()` - Core business logic for processing order messages
- **Domain**: Database persistence of order, item, and shipping entities

## API Contract (Kafka Message Consumer)

### Message Transport
- **Topic**: `orders`
- **Message Format**: Binary protobuf (`OrderResult` message)
- **Consumer Group**: `accounting`
- **Key**: `string` (unused)
- **Value**: `byte[]` (protobuf-serialized `OrderResult`)

### Payload Structure

The accounting service expects **binary protobuf** payloads containing complete order information including **order totals, item costs, and shipping charges**. Each message must be an `OrderResult` message defined in [`pb/demo.proto`](../../pb/demo.proto).

#### `OrderResult` Schema

```proto
message OrderResult {
  string   order_id               = 1;  // required - unique order identifier
  string   shipping_tracking_id   = 2;  // required - shipping carrier tracking ID
  Money    shipping_cost          = 3;  // required - total shipping cost
  Address  shipping_address       = 4;  // required - delivery address
  repeated OrderItem items        = 5;  // required - must contain at least one item
}
```

#### Supporting Message Types

```proto
message OrderItem {
  CartItem item  = 1; // required - product and quantity information
  Money    cost  = 2; // required - total cost for this line item (unit price × quantity)
}

message CartItem {
  string product_id = 1; // required - unique product identifier
  int32  quantity   = 2; // required - quantity ordered (must be > 0)
}

message Money {
  string currency_code = 1; // required - ISO-4217 currency code (e.g., "USD", "EUR")
  int64  units         = 2; // required - whole currency units (dollars, euros, etc.)
  int32  nanos         = 3; // required - fractional part in nanoseconds (0-999,999,999)
}

message Address {
  string street_address = 1; // required - street address
  string city           = 2; // required - city name
  string state          = 3; // required - state/province
  string country        = 4; // required - country name
  string zip_code       = 5; // required - postal/zip code
}
```

### Financial Data Processing

The accounting service extracts and persists the following financial information:

#### Order Totals
- **Item Costs**: Individual line item totals (`OrderItem.cost`)
- **Shipping Cost**: Delivery charges (`OrderResult.shipping_cost`)
- **Currency Information**: All monetary amounts include currency code and precise fractional values

#### Cost Calculation Examples
```jsonc
// Example: $25.99 per item × 2 quantity = $51.98 total
{
  "item": { "product_id": "SKU-123", "quantity": 2 },
  "cost": { "currency_code": "USD", "units": 51, "nanos": 980000000 }
}

// Example: $8.50 shipping cost
{
  "shipping_cost": { "currency_code": "USD", "units": 8, "nanos": 500000000 }
}
```

#### Payment Status
- The accounting service receives orders **after successful payment processing**
- All received orders represent **confirmed, paid transactions**
- No payment validation is performed (checkout service handles payment verification)

### Message Processing Behavior

#### Success Path
1. **Message Consumption**: Kafka message received on `orders` topic
2. **Protobuf Deserialization**: `OrderResult.Parser.ParseFrom(message.Value)`
3. **Database Persistence**: Creates three entity types:
   - `OrderEntity`: Order header with ID
   - `OrderItemEntity`: Line items with costs and product details
   - `ShippingEntity`: Shipping information and costs
4. **Transaction Commit**: All entities saved in a single database transaction

#### Error Handling
- **Parse Failures**: Invalid protobuf format logged and message skipped
- **Missing Fields**: Required field validation handled by protobuf parser
- **Database Errors**: Transaction rollback, error logged, message processing continues
- **No Message Rejection**: Failed messages are logged but not requeued

### Data Persistence Schema

The service persists order data across three database tables:

```sql
-- Order header information
OrderEntity {
  Id: string (order_id)
}

-- Individual line items with pricing
OrderItemEntity {
  ItemCostCurrencyCode: string,
  ItemCostUnits: long,
  ItemCostNanos: int,
  ProductId: string,
  Quantity: int,
  OrderId: string (FK)
}

-- Shipping details and costs
ShippingEntity {
  ShippingTrackingId: string,
  ShippingCostCurrencyCode: string,
  ShippingCostUnits: long,
  ShippingCostNanos: int,
  StreetAddress: string,
  City: string,
  State: string,
  Country: string,
  ZipCode: string,
  OrderId: string (FK)
}
```

### Example Complete Message

```jsonc
{
  "order_id": "550e8400-e29b-41d4-a716-446655440000",
  "shipping_tracking_id": "1Z999AA1234567890",
  "shipping_cost": {
    "currency_code": "USD",
    "units": 8,
    "nanos": 500000000  // $8.50 shipping
  },
  "shipping_address": {
    "street_address": "123 Main St",
    "city": "Anytown",
    "state": "CA",
    "country": "USA",
    "zip_code": "12345"
  },
  "items": [
    {
      "item": { "product_id": "PRODUCT-001", "quantity": 2 },
      "cost": {
        "currency_code": "USD",
        "units": 25,
        "nanos": 980000000  // $25.98 total (2 × $12.99)
      }
    },
    {
      "item": { "product_id": "PRODUCT-002", "quantity": 1 },
      "cost": {
        "currency_code": "USD",
        "units": 15,
        "nanos": 0  // $15.00 total
      }
    }
  ]
}
```

This message represents a **$49.48 order** ($25.98 + $15.00 + $8.50) with confirmed payment.

### Configuration Requirements

- **KAFKA_ADDR**: Kafka broker connection string (required)
- **DB_CONNECTION_STRING**: PostgreSQL connection string (optional - if not set, messages are logged but not persisted)

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
```
