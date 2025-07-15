// Shipping.gateway.spec.ts
// At the top of your test file, before any imports
process.env.SHIPPING_ADDR = 'http://localhost:8080';

import { Pact } from '@pact-foundation/pact';
import { Address, CartItem } from '../../protos/demo';
import ShippingGateway from './Shipping.gateway';

const pact = new Pact({
  consumer: 'Frontend',
  provider: 'ShippingService',
  port: 8080,
  log: process.cwd() + '/logs/pact.log',
  logLevel: 'info',
  dir: process.cwd() + '/pacts',
});

describe('Shipping Gateway', () => {
  // Make sure to await the setup promise
  beforeAll(async () => await pact.setup());
  
  // Also await the finalize promise
  afterAll(async () => await pact.finalize());
  
  // And the verify promise
  afterEach(async () => await pact.verify());

  describe('getShippingCost', () => {
    beforeEach(() => {
      return pact.addInteraction({
        state: 'shipping quote exists',
        uponReceiving: 'a request for shipping quote',
        withRequest: {
          method: 'POST',
          path: '/get-quote',
          headers: {
            'Content-Type': 'application/json'
          },
          body: {
            items: [
              {
                product_id: '1',
                quantity: 2
              }
            ],
            address: {
              street_address: '1600 Amphitheatre Parkway',
              city: 'Mountain View',
              state: 'CA',
              country: 'US',
              zip_code: '94043'
            }
          }
        },
        willRespondWith: {
          status: 200,
          headers: {
            'Content-Type': 'application/json'
          },
          body: {
            cost_usd: {
              currency_code: 'USD',
              units: 5,
              nanos: 990000000
            }
          }
        }
      });
    });

    it('returns shipping cost', async () => {
      // Environment setup for the test
      process.env.SHIPPING_ADDR = pact.mockService.baseUrl;
      // Add this right after setting the environment variable
      console.log('SHIPPING_ADDR:', process.env.SHIPPING_ADDR);
      
      const itemList: CartItem[] = [
        {
          productId: '1',
          quantity: 2
        } as CartItem
      ];

      const address: Address = {
        streetAddress: '1600 Amphitheatre Parkway',
        city: 'Mountain View',
        state: 'CA',
        country: 'US',
        zipCode: '94043'
      } as Address;

      const result = await ShippingGateway.getShippingCost(itemList, address);

      expect(result).toEqual({
        costUsd: {
          currencyCode: 'USD',
          units: 5,
          nanos: 990000000
        }
      });
    });
  });
});