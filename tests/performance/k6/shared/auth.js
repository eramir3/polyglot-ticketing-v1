import http from 'k6/http';
import { sleep } from 'k6';

const password = 'performance-ticket-password';

export function createVerifiedPerformanceUser({
  baseUrl,
  loadTestToken,
  mailpitUrl,
  setupEndpoint,
  userName,
  userPrefix,
}) {
  const email = `${userPrefix}-${runSuffix().toLowerCase()}@example.com`;

  assertSuccessfulResponse(
    http.post(
      `${baseUrl}/api/auth/signup`,
      JSON.stringify({ email, name: userName, password }),
      requestParameters(loadTestToken, setupEndpoint),
    ),
    201,
    'sign up the performance user',
  );

  const verificationEmail = waitForVerificationEmail({
    loadTestToken,
    mailpitUrl,
    recipient: email,
    setupEndpoint,
  });
  const verificationToken = verificationTokenFrom(verificationEmail);
  assertSuccessfulResponse(
    http.get(
      `${baseUrl}/api/auth/verify-email?token=${encodeURIComponent(verificationToken)}`,
      requestParameters(loadTestToken, setupEndpoint),
    ),
    200,
    'verify the performance user email',
  );

  const signInResponse = http.post(
    `${baseUrl}/api/auth/signin`,
    JSON.stringify({ email, password }),
    requestParameters(loadTestToken, setupEndpoint),
  );
  assertSuccessfulResponse(signInResponse, 201, 'sign in the performance user');

  const sessionCookie = sessionCookieFrom(signInResponse);
  const signInBody = signInResponse.json();
  if (
    !signInBody ||
    typeof signInBody !== 'object' ||
    typeof signInBody.user?.id !== 'string'
  ) {
    throw new Error('Sign-in did not return a user ID.');
  }

  return { sessionCookie, userId: signInBody.user.id };
}

function assertSuccessfulResponse(response, expectedStatus, action) {
  if (response.status !== expectedStatus) {
    throw new Error(
      `Unable to ${action}: expected HTTP ${expectedStatus}, received ${response.status}.`,
    );
  }
}

function requestParameters(loadTestToken, endpoint) {
  return {
    headers: {
      'Content-Type': 'application/json',
      'X-Ticketing-Load-Test-Token': loadTestToken,
    },
    tags: { endpoint },
  };
}

function runSuffix() {
  const runId = String(__ENV.K6_TEST_ID || Date.now()).replace(
    /[^a-zA-Z0-9]/g,
    '-',
  );
  return `${runId}-${Math.floor(Math.random() * 1_000_000)}`;
}

function sessionCookieFrom(response) {
  const session = response.cookies['better-auth.session_token']?.[0];
  if (!session || typeof session.value !== 'string' || session.value === '') {
    throw new Error('Sign-in did not return a session cookie.');
  }

  return `better-auth.session_token=${session.value}`;
}

function verificationTokenFrom(emailText) {
  const match = emailText.match(/\/api\/auth\/verify-email\?token=([^\s<"]+)/);
  if (!match) {
    throw new Error('Unable to find the email-verification token in Mailpit.');
  }

  return decodeURIComponent(match[1]);
}

function waitForVerificationEmail({
  loadTestToken,
  mailpitUrl,
  recipient,
  setupEndpoint,
}) {
  const timeoutAt = Date.now() + 10_000;
  while (Date.now() < timeoutAt) {
    const messagesResponse = http.get(
      `${mailpitUrl}/api/v1/messages`,
      requestParameters(loadTestToken, setupEndpoint),
    );
    assertSuccessfulResponse(
      messagesResponse,
      200,
      'retrieve performance-user emails from Mailpit',
    );

    const messages = messagesResponse.json();
    const message = messages.messages?.find((candidate) =>
      JSON.stringify(candidate).toLowerCase().includes(recipient.toLowerCase()),
    );
    if (message) {
      const detailResponse = http.get(
        `${mailpitUrl}/api/v1/message/${message.ID}`,
        requestParameters(loadTestToken, setupEndpoint),
      );
      assertSuccessfulResponse(
        detailResponse,
        200,
        'retrieve the performance-user verification email from Mailpit',
      );
      const email = detailResponse.json();
      if (!email || typeof email.Text !== 'string') {
        throw new Error(
          'Mailpit did not return the performance-user verification email text.',
        );
      }

      return email.Text;
    }

    sleep(0.1);
  }

  throw new Error(
    `Mailpit did not receive the verification email for ${recipient}.`,
  );
}
