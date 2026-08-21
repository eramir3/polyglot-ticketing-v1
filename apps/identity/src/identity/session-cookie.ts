const SESSION_COOKIE_NAME = 'better-auth.session_token';

export function extractSessionCookieValue(headers: Headers): string | null {
  const setCookieHeaders = getSetCookieHeaders(headers);
  const sessionCookie = setCookieHeaders.find((value) =>
    value.startsWith(`${SESSION_COOKIE_NAME}=`),
  );

  if (!sessionCookie) {
    return null;
  }

  const cookieValue = sessionCookie.slice(SESSION_COOKIE_NAME.length + 1);
  const separatorIndex = cookieValue.indexOf(';');

  return separatorIndex === -1
    ? cookieValue
    : cookieValue.slice(0, separatorIndex);
}

function getSetCookieHeaders(headers: Headers): string[] {
  const headersWithGetSetCookie = headers as Headers & {
    getSetCookie?: () => string[];
  };

  return (
    headersWithGetSetCookie.getSetCookie?.() ??
    (headers.get('set-cookie') ? [headers.get('set-cookie') as string] : [])
  );
}
