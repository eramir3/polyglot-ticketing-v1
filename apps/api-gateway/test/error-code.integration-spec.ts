import {
  ErrorCode as GatewayErrorCode,
  toPublicErrorCode as toGatewayPublicErrorCode,
} from '../src/errors/error-code';
import {
  ErrorCode as IdentityErrorCode,
  toPublicErrorCode as toIdentityPublicErrorCode,
} from '../../identity/src/errors/error-code';

describe('public error code conversion', () => {
  it('returns the public string for every defined error code', () => {
    const codes = Object.values(GatewayErrorCode).filter(
      (value): value is GatewayErrorCode =>
        typeof value === 'number' && value !== GatewayErrorCode.UNSPECIFIED,
    );

    for (const code of codes) {
      const expected = GatewayErrorCode[code];

      expect(toGatewayPublicErrorCode(code)).toBe(expected);
      expect(toIdentityPublicErrorCode(code as IdentityErrorCode)).toBe(
        expected,
      );
    }
  });

  it.each([GatewayErrorCode.UNSPECIFIED, 999 as GatewayErrorCode])(
    'falls back to INTERNAL_ERROR for code %s',
    (code) => {
      expect(toGatewayPublicErrorCode(code)).toBe('INTERNAL_ERROR');
      expect(toIdentityPublicErrorCode(code as IdentityErrorCode)).toBe(
        'INTERNAL_ERROR',
      );
    },
  );
});
