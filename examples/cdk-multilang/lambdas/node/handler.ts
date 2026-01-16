import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { KMSClient } from '@aws-sdk/client-kms';

import {
  AwsKmsEncryptionProvider,
  DynamormClient,
  DynamormError,
  defineModel,
} from '../../../../ts/dist/index.js';

const tableName = process.env.TABLE_NAME;
if (!tableName) {
  throw new Error('TABLE_NAME is required');
}

const kmsKeyArn = process.env.KMS_KEY_ARN;
if (!kmsKeyArn) {
  throw new Error('KMS_KEY_ARN is required');
}

const Demo = defineModel({
  name: 'DemoItem',
  table: { name: tableName },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'value', type: 'S', optional: true, omit_empty: true },
    { attribute: 'lang', type: 'S', optional: true, omit_empty: true },
    {
      attribute: 'secret',
      type: 'S',
      optional: true,
      omit_empty: true,
      encryption: { v: 1 },
    },
  ],
});

const ddb = new DynamoDBClient({ region: process.env.AWS_REGION });
const kms = new KMSClient({ region: process.env.AWS_REGION });
const encryption = new AwsKmsEncryptionProvider(kms, { keyArn: kmsKeyArn });
const db = new DynamormClient(ddb, { encryption }).register(Demo);

type LambdaEvent = {
  rawPath?: string;
  queryStringParameters?: Record<string, string>;
  requestContext?: { http?: { method?: string } };
  body?: string | null;
};

type LambdaResponse = {
  statusCode: number;
  headers?: Record<string, string>;
  body: string;
};

const jsonResponse = (statusCode: number, body: unknown): LambdaResponse => ({
  statusCode,
  headers: { 'content-type': 'application/json' },
  body: JSON.stringify(body),
});

export const handler = async (event: LambdaEvent): Promise<LambdaResponse> => {
  const method = event.requestContext?.http?.method ?? 'GET';
  const path = event.rawPath ?? '/';
  const qs = event.queryStringParameters ?? {};
  const parsedBody = event.body ? (JSON.parse(event.body) as Record<string, unknown>) : {};

  const pk = String(parsedBody.pk ?? qs.pk ?? '');
  const sk = String(parsedBody.sk ?? qs.sk ?? '');
  const value = String(parsedBody.value ?? qs.value ?? '');
  const secret = String(parsedBody.secret ?? qs.secret ?? '');
  const count = Number(parsedBody.count ?? qs.count ?? 0);
  const skPrefix = String(parsedBody.skPrefix ?? qs.skPrefix ?? '');

  if (path === '/batch') {
    if (method === 'GET') {
      return jsonResponse(405, { error: 'use POST/PUT' });
    }
    if (!pk) {
      return jsonResponse(400, { error: 'pk is required' });
    }

    const n = count > 0 ? count : 3;
    if (n > 25) {
      return jsonResponse(400, { error: 'count must be <= 25' });
    }

    const prefix = skPrefix || 'BATCH#';
    const puts = Array.from({ length: n }, (_, idx) => ({
      PK: pk,
      SK: `${prefix}${idx + 1}`,
      value,
      lang: 'ts',
      secret: secret || undefined,
    }));

    await db.batchWrite('DemoItem', { puts });
    const got = await db.batchGet(
      'DemoItem',
      puts.map((p) => ({ PK: p.PK, SK: p.SK })),
    );
    return jsonResponse(200, { ok: true, count: got.items.length, items: got.items });
  }

  if (path === '/tx') {
    if (method === 'GET') {
      return jsonResponse(405, { error: 'use POST/PUT' });
    }
    if (!pk) {
      return jsonResponse(400, { error: 'pk is required' });
    }

    const prefix = skPrefix || 'TX#';
    const item1 = { PK: pk, SK: `${prefix}1`, value, lang: 'ts', secret: secret || undefined };
    const item2 = { PK: pk, SK: `${prefix}2`, value, lang: 'ts', secret: secret || undefined };

    await db.transactWrite([
      { kind: 'put', model: 'DemoItem', item: item1 },
      { kind: 'put', model: 'DemoItem', item: item2 },
    ]);

    const got = await db.batchGet('DemoItem', [
      { PK: item1.PK, SK: item1.SK },
      { PK: item2.PK, SK: item2.SK },
    ]);
    return jsonResponse(200, { ok: true, count: got.items.length, items: got.items });
  }

  if (!pk || !sk) {
    return jsonResponse(400, { error: 'pk and sk are required' });
  }

  if (method === 'GET') {
    try {
      const item = await db.get('DemoItem', { PK: pk, SK: sk });
      return jsonResponse(200, { ok: true, item });
    } catch (err) {
      if (err instanceof DynamormError && err.code === 'ErrItemNotFound') {
        return jsonResponse(404, { error: 'not found' });
      }
      throw err;
    }
  }

  await db.create('DemoItem', { PK: pk, SK: sk, value, lang: 'ts', secret: secret || undefined });
  const item = await db.get('DemoItem', { PK: pk, SK: sk });
  return jsonResponse(200, { ok: true, item });
};
