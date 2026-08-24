import { Metadata } from '@grpc/grpc-js';

/**
 * Header allowlist mirrored from the API gateway's HTTP-to-gRPC conversion.
 * It carries browser request context only; session cookies and credentials do
 * not cross the gateway-to-identity boundary.
 */
const FORWARDED_AUTH_HEADERS = [
  'origin',
  'referer',
  'sec-fetch-dest',
  'sec-fetch-mode',
  'sec-fetch-site',
  'user-agent',
] as const;

/**
 * Converts incoming gRPC metadata back into the in-memory Web Headers object
 * expected by Better Auth. This does not create an HTTP request or route;
 * Identity remains a gRPC-only service.
 */
export function createAuthRequestHeaders(metadata: Metadata): Headers {
  const headers = new Headers();

  for (const headerName of FORWARDED_AUTH_HEADERS) {
    const value = metadata.get(headerName).find(isString);
    if (value !== undefined) {
      headers.set(headerName, value);
    }
  }

  return headers;
}

function isString(value: string | Buffer): value is string {
  return typeof value === 'string';
}
