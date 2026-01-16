import assert from 'node:assert/strict';

import {
  CreateTableCommand,
  DescribeTableCommand,
  DynamoDBClient,
  ListTablesCommand,
  ResourceInUseException,
} from '@aws-sdk/client-dynamodb';

import { DynamormClient } from '../../src/client.js';
import { DynamormError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';

const endpoint = process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000';

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? 'dummy',
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? 'dummy',
  },
});

try {
  await ddb.send(new ListTablesCommand({ Limit: 1 }));
  await ensureUsersTable(ddb);

  const user = defineModel({
    name: 'User',
    table: { name: 'users_contract' },
    naming: { convention: 'camelCase' },
    keys: {
      partition: { attribute: 'PK', type: 'S' },
      sort: { attribute: 'SK', type: 'S' },
    },
    attributes: [
      { attribute: 'PK', type: 'S', required: true, roles: ['pk'] },
      { attribute: 'SK', type: 'S', required: true, roles: ['sk'] },
      {
        attribute: 'createdAt',
        type: 'S',
        format: 'rfc3339nano',
        roles: ['created_at'],
      },
      {
        attribute: 'updatedAt',
        type: 'S',
        format: 'rfc3339nano',
        roles: ['updated_at'],
      },
      { attribute: 'version', type: 'N', format: 'int', roles: ['version'] },
    ],
  });

  const dynamorm = new DynamormClient(ddb).register(user);

  const pk = `USER#batch-${Date.now()}`;
  const keys = [
    { PK: pk, SK: '1' },
    { PK: pk, SK: '2' },
    { PK: pk, SK: '3' },
  ];

  const write = await dynamorm.batchWrite(
    'User',
    {
      puts: keys.map((k) => ({ ...k })),
    },
    { maxAttempts: 3, baseDelayMs: 5 },
  );
  assert.equal(write.unprocessed.length, 0);

  const got = await dynamorm.batchGet('User', keys, {
    maxAttempts: 3,
    baseDelayMs: 5,
  });
  assert.equal(got.unprocessedKeys.length, 0);
  assert.equal(got.items.length, 3);

  await dynamorm.batchWrite(
    'User',
    {
      deletes: keys,
    },
    { maxAttempts: 3, baseDelayMs: 5 },
  );

  const gotAfterDelete = await dynamorm.batchGet('User', keys, {
    maxAttempts: 3,
    baseDelayMs: 5,
  });
  assert.equal(gotAfterDelete.items.length, 0);

  const txKey = { PK: `${pk}#tx`, SK: 'PROFILE' };
  await dynamorm.transactWrite([
    { kind: 'put', model: 'User', item: txKey, ifNotExists: true },
  ]);

  await assert.rejects(
    () =>
      dynamorm.transactWrite([
        { kind: 'put', model: 'User', item: txKey, ifNotExists: true },
      ]),
    (err) => err instanceof DynamormError && err.code === 'ErrConditionFailed',
  );

  await assert.rejects(
    () =>
      dynamorm.transactWrite([
        {
          kind: 'condition',
          model: 'User',
          key: { PK: `${pk}#missing`, SK: 'PROFILE' },
          conditionExpression: 'attribute_exists(PK)',
        },
      ]),
    (err) => err instanceof DynamormError && err.code === 'ErrConditionFailed',
  );
} catch (err) {
  if (!process.env.CI) {
    console.warn(
      `Skipping batch/tx integration tests (endpoint unreachable: ${endpoint})`,
    );
    process.exit(0);
  }
  throw err;
} finally {
  ddb.destroy();
}

async function ensureUsersTable(client: DynamoDBClient): Promise<void> {
  const tableName = 'users_contract';
  try {
    await client.send(new DescribeTableCommand({ TableName: tableName }));
    return;
  } catch {
    // continue
  }

  try {
    await client.send(
      new CreateTableCommand({
        TableName: tableName,
        AttributeDefinitions: [
          { AttributeName: 'PK', AttributeType: 'S' },
          { AttributeName: 'SK', AttributeType: 'S' },
        ],
        KeySchema: [
          { AttributeName: 'PK', KeyType: 'HASH' },
          { AttributeName: 'SK', KeyType: 'RANGE' },
        ],
        ProvisionedThroughput: { ReadCapacityUnits: 1, WriteCapacityUnits: 1 },
      }),
    );
  } catch (err) {
    if (err instanceof ResourceInUseException) return;
    if (
      typeof err === 'object' &&
      err !== null &&
      'name' in err &&
      (err as { name?: unknown }).name === 'ResourceInUseException'
    ) {
      return;
    }
    throw err;
  }
}
