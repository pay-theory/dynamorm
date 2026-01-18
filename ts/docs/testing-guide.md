# Testing Guide (TypeScript)

This guide documents how to test TypeScript services that use `@pay-theory/dynamorm-ts`.

## Unit testing (recommended default)

Use the public testkit at `@pay-theory/dynamorm-ts/testkit` for strict AWS SDK v3 mocks.

```ts
import assert from 'node:assert/strict';
import { PutItemCommand } from '@aws-sdk/client-dynamodb';

import { DynamormClient } from '@pay-theory/dynamorm-ts';
import {
  createMockDynamoDBClient,
  fixedNow,
} from '@pay-theory/dynamorm-ts/testkit';

const mock = createMockDynamoDBClient();
mock.when(PutItemCommand, async () => ({}));

const db = new DynamormClient(mock.client, {
  now: fixedNow('2026-01-16T00:00:00.000000000Z'),
});
```

### ✅ CORRECT: strict command expectations

- Assert the expected AWS SDK command classes were sent.
- Prefer deterministic clocks (`fixedNow(...)`) for lifecycle fields.
- Prefer deterministic encryption providers for encrypted attributes.

## Integration testing (DynamoDB Local)

Use DynamoDB Local to validate real DynamoDB constraints (pagination, conditional writes, batch limits).

From repo root:

```bash
make docker-up
npm --prefix ts run test:integration
```

Required env vars (typical for local):

- `DYNAMODB_ENDPOINT` (default `http://localhost:8000`)
- `AWS_REGION` (default `us-east-1`)
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (use `dummy`)
