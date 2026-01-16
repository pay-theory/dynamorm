import assert from 'node:assert/strict';

import {
  QueryCommand,
  ScanCommand,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { encodeCursor } from '../../src/cursor.js';
import { DynamormClient } from '../../src/client.js';
import { DynamormError } from '../../src/errors.js';
import { defineModel } from '../../src/model.js';

class StubDdb {
  calls = 0;
  last: unknown | undefined;
  constructor(
    private readonly handler: (cmd: unknown, call: number) => unknown,
  ) {}
  async send(cmd: unknown): Promise<unknown> {
    this.calls += 1;
    this.last = cmd;
    return this.handler(cmd, this.calls);
  }
}

const User = defineModel({
  name: 'User',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'emailHash', type: 'S', optional: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
  indexes: [
    {
      name: 'gsi-email',
      type: 'GSI',
      partition: { attribute: 'emailHash', type: 'S' },
      projection: { type: 'ALL' },
    },
  ],
});

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').page(),
    (e) => e instanceof DynamormError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () =>
      client
        .query('User')
        .usingIndex('gsi-email')
        .consistentRead()
        .partitionKey('x')
        .page(),
    (e) => e instanceof DynamormError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').partitionKey('A').sortKey('between', '1').page(),
    (e) => e instanceof DynamormError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof QueryCommand) {
      return {
        Items: [{ PK: { S: 'A' }, SK: { S: '1' }, version: { N: '0' } }],
        LastEvaluatedKey: { PK: { S: 'A' }, SK: { S: '1' } },
      };
    }
    throw new Error('unexpected');
  });
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client.query('User').partitionKey('A').limit(1).page();
  assert.equal(page.items.length, 1);
  assert.ok(page.cursor);
  assert.ok(ddb.last instanceof QueryCommand);
  assert.equal(ddb.last.input.ScanIndexForward, true);
}

{
  const cursor = encodeCursor({
    lastKey: { PK: { S: 'A' }, SK: { S: '1' } },
    index: 'gsi-email',
    sort: 'ASC',
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.query('User').partitionKey('A').cursor(cursor).page(),
    (e) => e instanceof DynamormError && e.code === 'ErrInvalidOperator',
  );
}

{
  const ddb = new StubDdb((cmd) => {
    if (cmd instanceof ScanCommand) {
      return {
        Items: [],
        LastEvaluatedKey: { PK: { S: 'A' }, SK: { S: '1' } },
      };
    }
    throw new Error('unexpected');
  });
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  const page = await client.scan('User').limit(1).page();
  assert.equal(page.items.length, 0);
  assert.ok(page.cursor);
  assert.ok(ddb.last instanceof ScanCommand);
}

{
  const cursor = encodeCursor({
    lastKey: { PK: { S: 'A' }, SK: { S: '1' } },
    index: 'other',
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    User,
  );
  await assert.rejects(
    () => client.scan('User').usingIndex('gsi-email').cursor(cursor).page(),
    (e) => e instanceof DynamormError && e.code === 'ErrInvalidOperator',
  );
}

{
  const enc = defineModel({
    name: 'Enc',
    table: { name: 'enc_contract' },
    keys: { partition: { attribute: 'PK', type: 'S' } },
    attributes: [
      { attribute: 'PK', type: 'S', roles: ['pk'] },
      { attribute: 'secret', type: 'S', encryption: { v: 1 } },
    ],
  });
  const ddb = new StubDdb(() => ({ Items: [] }));
  const client = new DynamormClient(ddb as unknown as DynamoDBClient).register(
    enc,
  );
  await assert.rejects(
    () => client.query('Enc').partitionKey('A').page(),
    (e) =>
      e instanceof DynamormError && e.code === 'ErrEncryptionNotConfigured',
  );
}
