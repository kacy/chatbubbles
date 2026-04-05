import type { PairPayload } from './types';

export function parsePairPayload(raw: string): PairPayload {
  const parsed = JSON.parse(raw.trim()) as Partial<PairPayload>;

  if (!parsed.h || !parsed.fp || !parsed.c || typeof parsed.v !== 'number') {
    throw new Error('pair payload is missing required fields');
  }

  return {
    h: String(parsed.h),
    fp: String(parsed.fp),
    c: String(parsed.c),
    v: parsed.v,
  };
}
