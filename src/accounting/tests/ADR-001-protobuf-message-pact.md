# ADR-001: Protobuf Message Pact Testing Work-around in C#

*Status*: Accepted 2025-07-16  
*Component*: `src/accounting/tests`  
*Authors*: AI pair-programming session (Cascade)

---

## Context

The **Accounting** service consumes a Kafka topic (`orders`) where each value is a
`oteldemo.OrderResult` protobuf message.  We want a consumer-side **Pact
Message** test so that downstream services can verify the message contract in a
Pact broker.

### The problem with PactNet & Protobuf (July 2025)

1. **No plugin support in PactNet** – The C# binding (PactNet ≥ 5.0.1) does not
   yet expose the [Pact Plugin](https://github.com/pact-foundation/pact-plugins)
   FFI needed to handle binary codecs such as Protobuf (see pact-net issue
   #489 & #492).
2. `IMessagePactBuilderV4` only offers `.WithJsonContent(...)`; there is no API
   for arbitrary byte arrays.
3. The official Pact **Protobuf plugin** *does* exist, but is only usable via
   the Pact CLI or via languages that already support plugins (Ruby, JS, Go »
   not C# at the moment).

### Options considered

| # | Approach | Pros | Cons |
|---|-----------|------|------|
| A | **JSON façade** – Represent the proto as JSON in the pact definition, then convert JSON ⇒ Protobuf inside the verification lambda. | • Works entirely inside C# / .NET.<br>• Keeps Pact flows (publish/verify) unchanged.<br>• Exercises real consumer code by re-serialising to binary. | • Does **not** verify wire-level bytes.<br>• Duplicate definition of message structure in JSON.<br>• Requires extra parsing code in the test. |
| B | Run **Pact CLI + Protobuf plugin** from an external process and check results in C#. | • Full binary verification.<br>• Future-proof – same plugin used by other languages. | • Extra tooling/deployment complexity.<br>• Can’t be run inside pure `dotnet test` easily.<br>• Windows developers need additional binaries. |
| C | **Wait** for PactNet plugin support. | • Cleanest long-term solution. | • Blocks us today; unknown delivery date. |

### Decision

We chose **Option A (JSON façade)** because:

* It unblocks us immediately with **zero external tooling**.
* The critical contract is the *logical field set*, not the wire encoding;
  binary drift is low-risk because both consumer & producer share the `.proto`.
* We still exercise the **real** `Consumer.ProcessMessage` path by converting
  the JSON to binary bytes before invoking it.

If/when PactNet gains plugin support we can migrate with minimal changes (swap
`WithJsonContent` for the plugin call).

---

## Detailed Breakdown – `ConsumerMessagePactTests`

### High-level flow

```text
Pact builder → JSON contract → Pact reifies JSON → Test turns JSON → protobuf bytes →
Kafka Message object → Consumer.ProcessMessage (via reflection)
```

### Step-by-step

1. **Test fixture** (`ConsumerMessagePactTests` constructor)
   * Sets dummy env vars (`KAFKA_ADDR`) to satisfy production code paths.
   * Creates a V4 pact instance: `Pact.V4("accounting-consumer", "checkout-provider", …)`.
   * Calls `.WithMessageInteractions()` and stores it.

2. **Defining the expected message**

   ```csharp
   _messagePact
       .ExpectsToReceive("order-result message")
       .WithMetadata("contentType", "application/json")
       .WithJsonContent(new { … })
   ```

   * The **JSON body** mirrors the `OrderResult` schema:
     * All primitive types are literal values (`orderId` as string, `units` as
       `int64` → JSON number etc.).
     * Matchers were removed to avoid proto-JSON coercion issues.

3. **Verification callback** (`Verify<JsonElement>(json ⇒ { … })`)

   1. Pact passes the *reified* JSON document (a `JsonElement`).
   2. We extract the string JSON (`element.GetRawText()`).
   3. `Google.Protobuf.JsonParser` parses it into a **real**
      `OrderResult` object (`parser.Parse<OrderResult>(json)`).
   4. We serialise back to **binary bytes** (`order.ToByteArray()`), matching
      the bytes that would be on Kafka.

4. **Calling the production code**

   * `Consumer` is *internal*, so we obtain its `Type` via
     `Type.GetType("Accounting.Consumer, Accounting")`.
   * `Consumer` requires an `ILogger<Consumer>`.  We obtain a **no-op logger**
     via reflection (`NullLogger<Consumer>.Instance` – static **field**).
   * We instantiate the consumer: `Activator.CreateInstance(consumerType, logger)`.

5. **Building a fake Kafka message**

   * `new Confluent.Kafka.Message<string, byte[]>()` is created via reflection
     and populated with `Key = ""` and `Value = protoBytes`.

6. **Invoke `ProcessMessage`**

   * Using reflection to access the private method:
     `consumerType.GetMethod("ProcessMessage", BindingFlags.Instance|BindingFlags.NonPublic)`.
   * Pass the fake Kafka message – this exercises *all* parsing & EF Core logic
     exactly as in production.

7. **Assertions**

   * We rely on the method **not throwing**; any exception bubbles and fails
     Pact’s verification (`PactMessageConsumerVerificationException`).
   * Additional logical assertions can be added later (e.g. inspect EF Core in-memory DB).

8. **Outcome**

   * Pact writes `accounting-consumer-checkout-provider.json` under
     `../../pacts/` so provider teams can verify.

---

## Consequences & Next Steps

* Pact contract verifies the logical schema; binary wire compatibility rests on
  shared proto.
* When PactNet plugin support lands we can:  
  `WithPlugin("protobuf") → WithBinaryContent(bytes)` and drop the JSON façade.
* CI can now publish pact files to our Pact Broker, enabling provider
  verification gates.
