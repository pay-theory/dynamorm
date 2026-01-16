import { DynamoDBClient } from '@aws-sdk/client-dynamodb';

import { DynamormClient, defineModel } from '../../../../ts/dist/index.js';

const tableName = process.env.TABLE_NAME;
if (!tableName) {
  throw new Error('TABLE_NAME is required');
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
  ],
});

const ddb = new DynamoDBClient({ region: process.env.AWS_REGION });
const db = new DynamormClient(ddb).register(Demo);

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
  const qs = event.queryStringParameters ?? {};
  const parsedBody = event.body ? (JSON.parse(event.body) as Record<string, unknown>) : {};

  const pk = String(parsedBody.pk ?? qs.pk ?? '');
  const sk = String(parsedBody.sk ?? qs.sk ?? '');
  const value = String(parsedBody.value ?? qs.value ?? '');

  if (!pk || !sk) {
    return jsonResponse(400, { error: 'pk and sk are required' });
  }

  if (method === 'GET') {
    const item = await db.get('DemoItem', { PK: pk, SK: sk });
    return jsonResponse(200, { ok: true, item });
  }

  await db.create('DemoItem', { PK: pk, SK: sk, value, lang: 'ts' });
  const item = await db.get('DemoItem', { PK: pk, SK: sk });
  return jsonResponse(200, { ok: true, item });
};
