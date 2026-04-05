import { describe, expect, test } from 'vitest';

import { parsePairPayload } from './qr';

describe('pair payload parsing', () => {
  test('parses a valid payload', () => {
    expect(
      parsePairPayload('{"h":"100.64.0.3:8443","fp":"SHA256:test","c":"ABC123","v":1}'),
    ).toEqual({
      h: '100.64.0.3:8443',
      fp: 'SHA256:test',
      c: 'ABC123',
      v: 1,
    });
  });

  test('rejects a payload missing required fields', () => {
    expect(() => parsePairPayload('{"h":"100.64.0.3:8443"}')).toThrow(
      'pair payload is missing required fields',
    );
  });
});
