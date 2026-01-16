import { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { DynamormClient, defineModel } from '../../src/index.js';

const User = defineModel({
  name: 'User',
  table: { name: process.env.USERS_TABLE ?? 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

let db: DynamormClient | undefined;

function getDb(): DynamormClient {
  if (db) return db;

  const ddb = new DynamoDBClient({
    region: process.env.AWS_REGION,
    endpoint: process.env.DYNAMODB_ENDPOINT,
  });
  db = new DynamormClient(ddb).register(User);
  return db;
}

export const handler = async (event: { pk: string; sk: string }) => {
  const client = getDb();
  const item = await client.get('User', { PK: event.pk, SK: event.sk });
  return { ok: true, item };
};
