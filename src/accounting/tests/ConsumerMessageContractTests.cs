using System.Reflection;
using Accounting;
using Confluent.Kafka;
using Google.Protobuf;
using Microsoft.Extensions.Logging.Abstractions;
using Oteldemo;
using PactNet;
using PactNet.Verifier;
using Xunit;

namespace Accounting.Tests;

public class ConsumerMessageContractTests
{
    private const string PactDir = "pacts";

    [Fact]
    public void ProcessMessage_handles_valid_OrderResult_message()
    {
        // Build a sample OrderResult matching the protobuf schema
        var sampleBytes = BuildSamplePayload();

        // Configure Pact message verification
        var pact = MessagePact.V3("accounting-consumer", "checkout-provider", cfg =>
        {
            cfg.PactDir(PactDir);
        });

        pact.Messages(m =>
        {
            m.ExpectsToReceive("a valid OrderResult message")
             .WithMetadata(new Dictionary<string, object>
             {
                 { "contentType", "application/protobuf" }
             })
             .WithBinaryContent(sampleBytes);
        });

        pact.Verify(bytes =>
        {
            // Arrange a Consumer instance with a null DBContext so we skip persistence.
            var consumer = new Consumer(NullLogger<Consumer>.Instance);
            // set _dbContext -> null using reflection (private field)
            typeof(Consumer)
                .GetField("_dbContext", BindingFlags.Instance | BindingFlags.NonPublic)!
                .SetValue(consumer, null);

            // Act – invoke handler directly
            consumer.ProcessMessage(new Message<string, byte[]> { Value = bytes });
        });
    }

    private static byte[] BuildSamplePayload()
    {
        var order = new OrderResult
        {
            OrderId = "2",
            ShippingTrackingId = "1234567890",
            ShippingCost = new Money { CurrencyCode = "USD", Units = 100, Nanos = 0 },
            ShippingAddress = new Address
            {
                StreetAddress = "123 Main St",
                City = "Anytown",
                State = "CA",
                Country = "USA",
                ZipCode = "12345"
            }
        };

        order.Items.Add(new OrderItem
        {
            Item = new CartItem { ProductId = "1", Quantity = 2 },
            Cost = new Money { CurrencyCode = "USD", Units = 100, Nanos = 0 }
        });

        return order.ToByteArray();
    }
}
