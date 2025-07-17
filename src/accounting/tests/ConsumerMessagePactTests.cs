using System.Text.Json;
using Google.Protobuf;
using PactNet;
using PactNet.Matchers;
using Xunit.Abstractions;
using Xunit;
using Oteldemo;
using Microsoft.Extensions.Logging.Abstractions;
using Accounting;

namespace tests;

public class ConsumerMessagePactTests : IDisposable
{
    private readonly IMessagePactBuilderV4 _messagePact;

    public ConsumerMessagePactTests(ITestOutputHelper output)
    {
        // Ensure dummy env vars so the Accounting consumer code (if invoked) does not throw
        Environment.SetEnvironmentVariable("KAFKA_ADDR", "localhost:9092");

        var pact = Pact.V4(
            "accounting-consumer",
            "checkout-provider",
            new PactConfig
            {
                PactDir = "../../../pacts",
                DefaultJsonSettings = new JsonSerializerOptions
                {
                    PropertyNamingPolicy = JsonNamingPolicy.CamelCase
                },
                // No explicit outputter needed
            });

        _messagePact = pact.WithMessageInteractions();
    }

    [Fact]
    public void Process_order_result_message()
    {
        _messagePact
            .ExpectsToReceive("order-result message")
            .Given("An order has been successfully processed")
            .WithMetadata("contentType", "application/json")
            .WithJsonContent(new
            {
                orderId = "123",
                shippingTrackingId = "trk-1",
                shippingCost = new
                {
                    currencyCode = "USD",
                    units = 5,
                    nanos = 0
                },
                shippingAddress = new
                {
                    streetAddress = "123 Main St",
                    city = "Anytown",
                    state = "CA",
                    country = "USA",
                    zipCode = "94016"
                },
                items = new[]
                {
                    new
                    {
                        item = new
                        {
                            productId = "SKU-1",
                            quantity = 2
                        },
                        cost = new
                        {
                            currencyCode = "USD",
                            units = 3,
                            nanos = 0
                        }
                    }
                }
            })
            .Verify<JsonElement>(jsonElement =>
            {
                // Parse the JSON that Pact passes back into OrderResult using protobuf's JsonParser
                var parser = new JsonParser(JsonParser.Settings.Default.WithIgnoreUnknownFields(true));
                var jsonBody = jsonElement.GetRawText();
                Console.WriteLine("Reified JSON:\n" + jsonBody);
                var order = parser.Parse<OrderResult>(jsonBody);

                // Build proto bytes equivalent to what Kafka would carry
                var protoBytes = order.ToByteArray();

                // Instantiate Consumer with a no-op logger
                // Obtain Consumer type via reflection (it's internal)
                var consumerType = Type.GetType("Accounting.Consumer, Accounting")!;
                var loggerFactory = NullLoggerFactory.Instance;
                var loggerGeneric = typeof(NullLogger<>).MakeGenericType(consumerType);
                var loggerField = loggerGeneric.GetField("Instance", System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.Static);
                var logger = loggerField!.GetValue(null);

                using var consumer = (IDisposable?)Activator.CreateInstance(consumerType, logger!);

                var kafkaMsgType = typeof(Confluent.Kafka.Message<,>).MakeGenericType(typeof(string), typeof(byte[]));
                var kafkaMsg = Activator.CreateInstance(kafkaMsgType)!;
                kafkaMsgType.GetProperty("Key")!.SetValue(kafkaMsg, string.Empty);
                kafkaMsgType.GetProperty("Value")!.SetValue(kafkaMsg, protoBytes);

                // Invoke private ProcessMessage using reflection so we exercise the real parsing logic
                var processMethod = consumerType.GetMethod("ProcessMessage", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic);
                processMethod!.Invoke(consumer, new[] { kafkaMsg });

                // If ProcessMessage throws, the test will fail. Basic sanity: original orderId still correct.
                // Sanity check omitted; main purpose is ensuring no exception from ProcessMessage
            });
    }

    public void Dispose()
    {
        // Pact files are written automatically when the test completes successfully
        // No explicit cleanup needed
    }
}
