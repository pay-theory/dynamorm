import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import type { AttributeSchema, Model } from './model.js';
import { DynamormError } from './errors.js';

export function nowRfc3339Nano(date = new Date()): string {
  const iso = date.toISOString(); // always has milliseconds: YYYY-MM-DDTHH:mm:ss.sssZ
  return iso.replace(/\.(\d{3})Z$/, '.$1000000Z');
}

export function isEmpty(value: unknown): boolean {
  if (value === null || value === undefined) return true;

  if (typeof value === 'string') return value.length === 0;
  if (typeof value === 'number') return value === 0;
  if (typeof value === 'boolean') return value === false;

  if (value instanceof Date) return Number.isNaN(value.getTime());

  if (Array.isArray(value)) return value.length === 0;

  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) return true;
    return entries.every(([, v]) => isEmpty(v));
  }

  return false;
}

export function marshalKey(
  model: Model,
  key: Record<string, unknown>,
): Record<string, AttributeValue> {
  const pkName = model.roles.pk;
  const pkSchema = model.attributes.get(pkName);
  if (!pkSchema)
    throw new DynamormError(
      'ErrInvalidModel',
      `Model missing pk attribute schema: ${pkName}`,
    );

  const out: Record<string, AttributeValue> = {};

  const pkValue = key[pkName];
  if (isEmpty(pkValue))
    throw new DynamormError(
      'ErrMissingPrimaryKey',
      `Missing partition key: ${pkName}`,
    );
  out[pkName] = marshalScalar(pkSchema, pkValue);

  if (model.roles.sk) {
    const skName = model.roles.sk;
    const skSchema = model.attributes.get(skName);
    if (!skSchema)
      throw new DynamormError(
        'ErrInvalidModel',
        `Model missing sk attribute schema: ${skName}`,
      );
    const skValue = key[skName];
    if (isEmpty(skValue))
      throw new DynamormError(
        'ErrMissingPrimaryKey',
        `Missing sort key: ${skName}`,
      );
    out[skName] = marshalScalar(skSchema, skValue);
  }

  return out;
}

export function marshalPutItem(
  model: Model,
  item: Record<string, unknown>,
  opts: { now?: string } = {},
): Record<string, AttributeValue> {
  const now = opts.now ?? nowRfc3339Nano();

  const knownAttributes = new Set(
    model.schema.attributes.map((a) => a.attribute),
  );
  for (const key of Object.keys(item)) {
    if (!knownAttributes.has(key)) {
      throw new DynamormError(
        'ErrInvalidModel',
        `Unknown attribute for model ${model.name}: ${key}`,
      );
    }
  }

  const out: Record<string, AttributeValue> = {};

  // Enforce keys exist.
  Object.assign(out, marshalKey(model, item));

  for (const attr of model.schema.attributes) {
    const name = attr.attribute;
    if (name === model.roles.pk || name === model.roles.sk) continue;

    // Lifecycle fields.
    if (name === model.roles.createdAt) {
      out[name] = { S: now };
      continue;
    }
    if (name === model.roles.updatedAt) {
      out[name] = { S: now };
      continue;
    }
    if (name === model.roles.version) {
      const v = item[name];
      out[name] = { N: String(isEmpty(v) ? 0 : (v as number)) };
      continue;
    }

    const value = item[name];
    if (value === undefined) continue;

    if (attr.omit_empty && isEmpty(value)) continue;

    out[name] = marshalScalar(attr, value);
  }

  return out;
}

export function unmarshalItem(
  model: Model,
  item: Record<string, AttributeValue>,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};

  for (const [name, av] of Object.entries(item)) {
    const schema = model.attributes.get(name);
    if (!schema) {
      out[name] = av;
      continue;
    }
    out[name] = unmarshalScalar(schema, av);
  }

  return out;
}

export function marshalScalar(
  schema: AttributeSchema,
  value: unknown,
): AttributeValue {
  switch (schema.type) {
    case 'S':
      if (typeof value !== 'string')
        throw new DynamormError(
          'ErrInvalidModel',
          `Expected string for ${schema.attribute}`,
        );
      return { S: value };
    case 'N':
      if (typeof value === 'number') return { N: String(value) };
      if (typeof value === 'bigint') return { N: value.toString() };
      if (typeof value === 'string') return { N: value };
      throw new DynamormError(
        'ErrInvalidModel',
        `Expected number for ${schema.attribute}`,
      );
    case 'SS': {
      if (!Array.isArray(value)) {
        throw new DynamormError(
          'ErrInvalidModel',
          `Expected string[] for ${schema.attribute}`,
        );
      }
      const ss = value.map((v) => {
        if (typeof v !== 'string') {
          throw new DynamormError(
            'ErrInvalidModel',
            `Expected string[] for ${schema.attribute}`,
          );
        }
        return v;
      });
      if (ss.length === 0) return { NULL: true };
      return { SS: ss };
    }
    case 'BOOL':
      if (typeof value !== 'boolean')
        throw new DynamormError(
          'ErrInvalidModel',
          `Expected boolean for ${schema.attribute}`,
        );
      return { BOOL: value };
    case 'NULL':
      return { NULL: true };
    default:
      throw new DynamormError(
        'ErrInvalidModel',
        `Unsupported type ${schema.type} for ${schema.attribute}`,
      );
  }
}

export function unmarshalScalar(
  schema: AttributeSchema,
  av: AttributeValue,
): unknown {
  if ('S' in av && av.S !== undefined) return av.S;
  if ('N' in av && av.N !== undefined) return Number(av.N);
  if ('SS' in av && av.SS !== undefined) return av.SS.slice();
  if ('BOOL' in av && av.BOOL !== undefined) return av.BOOL;
  if ('NULL' in av && av.NULL) return null;

  throw new DynamormError(
    'ErrInvalidModel',
    `Unsupported AttributeValue for ${schema.attribute}`,
  );
}
