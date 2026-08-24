import { Metadata } from '@grpc/grpc-js';
import { IncomingHttpHeaders } from 'node:http';
import { SESSION_COOKIE_NAME } from '../identity.constants';

/**
 * Converts only the Better Auth session cookie from an HTTP request into gRPC
 * metadata. Other browser cookies must not cross the gateway-to-identity
 * boundary.
 */
export function createSessionRequestMetadata(
  headers: IncomingHttpHeaders,
): Metadata {
  const metadata = new Metadata();
  const sessionCookieValue = getCookieValue(
    headers.cookie,
    SESSION_COOKIE_NAME,
  );

  if (sessionCookieValue !== undefined) {
    metadata.set('cookie', `${SESSION_COOKIE_NAME}=${sessionCookieValue}`);
  }

  return metadata;
}

function getCookieValue(
  cookieHeader: string | string[] | undefined,
  cookieName: string,
): string | undefined {
  const cookieValue = Array.isArray(cookieHeader)
    ? cookieHeader.join(';')
    : cookieHeader;

  if (!cookieValue) {
    return undefined;
  }

  for (const cookie of cookieValue.split(';')) {
    const [name, ...valueParts] = cookie.trim().split('=');
    if (name === cookieName) {
      return valueParts.join('=');
    }
  }

  return undefined;
}
