import assert from 'node:assert/strict';
import { DynamoDBClient, ListTablesCommand } from '@aws-sdk/client-dynamodb';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';

const client = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

try {
  const resp = await client.send(new ListTablesCommand({ Limit: 1 }));
  assert.ok(resp.TableNames !== undefined);
} catch (err) {
  if (!process.env.CI) {
    // Local dev convenience: allow running unit tests without DynamoDB Local.
    console.warn(
      `Skipping DynamoDB Local integration test (endpoint unreachable: ${endpoint})`,
    );
    process.exit(0);
  }
  throw err;
} finally {
  client.destroy();
}
