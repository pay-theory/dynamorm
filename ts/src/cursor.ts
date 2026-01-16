import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { DynamormError } from './errors.js';

export type CursorSort = 'ASC' | 'DESC';

export interface Cursor {
  lastKey: Record<string, AttributeValue>;
  index?: string;
  sort?: CursorSort;
}

export function encodeCursor(cursor: Cursor): string {
  if (!cursor?.lastKey || typeof cursor.lastKey !== 'object') {
    throw new DynamormError('ErrInvalidModel', 'Cursor lastKey is required');
  }

  const lastKeyJson: Record<string, unknown> = {};
  for (const key of Object.keys(cursor.lastKey).sort()) {
    const av = cursor.lastKey[key];
    if (!av)
      throw new DynamormError(
        'ErrInvalidModel',
        `Cursor lastKey missing value: ${key}`,
      );
    lastKeyJson[key] = toDmsAttributeValue(av);
  }

  const parts: string[] = [];
  parts.push(`"lastKey":${stableStringify(lastKeyJson)}`);
  if (cursor.index) parts.push(`"index":${JSON.stringify(cursor.index)}`);
  if (cursor.sort) parts.push(`"sort":${JSON.stringify(cursor.sort)}`);

  const json = `{${parts.join(',')}}`;
  return base64UrlEncode(Buffer.from(json, 'utf8'));
}

export function decodeCursor(encoded: string): Cursor {
  const raw = String(encoded ?? '').trim();
  if (!raw)
    throw new DynamormError('ErrInvalidModel', 'Cursor string is empty');

  const jsonStr = base64UrlDecode(raw).toString('utf8');
  const parsed = JSON.parse(jsonStr) as {
    lastKey?: unknown;
    index?: unknown;
    sort?: unknown;
  };

  if (!parsed || typeof parsed !== 'object')
    throw new DynamormError('ErrInvalidModel', 'Cursor JSON is invalid');

  const lastKeyObj = parsed.lastKey;
  if (
    !lastKeyObj ||
    typeof lastKeyObj !== 'object' ||
    Array.isArray(lastKeyObj)
  ) {
    throw new DynamormError('ErrInvalidModel', 'Cursor lastKey is invalid');
  }

  const lastKey: Record<string, AttributeValue> = {};
  for (const key of Object.keys(lastKeyObj as Record<string, unknown>)) {
    lastKey[key] = fromDmsAttributeValue(
      (lastKeyObj as Record<string, unknown>)[key],
    );
  }

  const out: Cursor = { lastKey };
  if (typeof parsed.index === 'string') out.index = parsed.index;
  if (parsed.sort === 'ASC' || parsed.sort === 'DESC') out.sort = parsed.sort;

  return out;
}

function base64UrlEncode(buf: Buffer): string {
  return buf.toString('base64').replace(/\+/g, '-').replace(/\//g, '_');
}

function base64UrlDecode(input: string): Buffer {
  const b64 = input.replace(/-/g, '+').replace(/_/g, '/');
  return Buffer.from(b64, 'base64');
}

function toDmsAttributeValue(av: AttributeValue): unknown {
  if ('S' in av && av.S !== undefined) return { S: av.S };
  if ('N' in av && av.N !== undefined) return { N: av.N };
  if ('BOOL' in av && av.BOOL !== undefined) return { BOOL: av.BOOL };
  if ('NULL' in av && av.NULL !== undefined) return { NULL: av.NULL };
  if ('SS' in av && av.SS !== undefined) return { SS: av.SS };
  if ('NS' in av && av.NS !== undefined) return { NS: av.NS };
  if ('B' in av && av.B !== undefined)
    return { B: Buffer.from(av.B).toString('base64') };
  if ('BS' in av && av.BS !== undefined)
    return { BS: av.BS.map((b) => Buffer.from(b).toString('base64')) };
  if ('L' in av && av.L !== undefined)
    return { L: av.L.map(toDmsAttributeValue) };
  if ('M' in av && av.M !== undefined) {
    const out: Record<string, unknown> = {};
    for (const key of Object.keys(av.M).sort()) {
      const child = av.M[key];
      if (!child)
        throw new DynamormError(
          'ErrInvalidModel',
          `Invalid cursor map value: ${key}`,
        );
      out[key] = toDmsAttributeValue(child);
    }
    return { M: out };
  }

  throw new DynamormError(
    'ErrInvalidModel',
    'Invalid AttributeValue in cursor',
  );
}

function fromDmsAttributeValue(input: unknown): AttributeValue {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    throw new DynamormError(
      'ErrInvalidModel',
      'Invalid cursor AttributeValue JSON',
    );
  }

  const keys = Object.keys(input as Record<string, unknown>);
  if (keys.length !== 1)
    throw new DynamormError(
      'ErrInvalidModel',
      'Invalid cursor AttributeValue JSON',
    );
  const k = keys[0]!;
  const v = (input as Record<string, unknown>)[k];

  switch (k) {
    case 'S':
      if (typeof v !== 'string')
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor S value');
      return { S: v };
    case 'N':
      if (typeof v !== 'string')
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor N value');
      return { N: v };
    case 'BOOL':
      if (typeof v !== 'boolean')
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor BOOL value');
      return { BOOL: v };
    case 'NULL':
      if (v !== true)
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor NULL value');
      return { NULL: true };
    case 'SS':
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string')) {
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor SS value');
      }
      return { SS: v as string[] };
    case 'NS':
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string')) {
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor NS value');
      }
      return { NS: v as string[] };
    case 'B':
      if (typeof v !== 'string')
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor B value');
      return { B: Buffer.from(v, 'base64') };
    case 'BS':
      if (!Array.isArray(v) || v.some((x) => typeof x !== 'string')) {
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor BS value');
      }
      return { BS: (v as string[]).map((s) => Buffer.from(s, 'base64')) };
    case 'L':
      if (!Array.isArray(v))
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor L value');
      return { L: (v as unknown[]).map(fromDmsAttributeValue) };
    case 'M': {
      if (!v || typeof v !== 'object' || Array.isArray(v)) {
        throw new DynamormError('ErrInvalidModel', 'Invalid cursor M value');
      }
      const out: Record<string, AttributeValue> = {};
      for (const key of Object.keys(v as Record<string, unknown>)) {
        out[key] = fromDmsAttributeValue((v as Record<string, unknown>)[key]);
      }
      return { M: out };
    }
    default:
      throw new DynamormError(
        'ErrInvalidModel',
        `Unsupported cursor AttributeValue type: ${k}`,
      );
  }
}

function stableStringify(value: unknown): string {
  if (value === undefined) return 'null';
  if (value === null) return 'null';
  if (typeof value === 'string') return JSON.stringify(value);
  if (typeof value === 'number') return JSON.stringify(value);
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;

  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj)
      .filter((k) => obj[k] !== undefined)
      .sort();
    const parts = keys.map(
      (k) => `${JSON.stringify(k)}:${stableStringify(obj[k])}`,
    );
    return `{${parts.join(',')}}`;
  }

  return JSON.stringify(value);
}
