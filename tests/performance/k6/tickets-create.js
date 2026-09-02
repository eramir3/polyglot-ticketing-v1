import http from 'k6/http';
import { check, sleep } from 'k6';

const baseUrl = __ENV.K6_BASE_URL || 'http://api-gateway:3000';
const mailpitUrl = __ENV.K6_MAILPIT_URL || 'http://mailpit:8025';
const testConfigs = JSON.parse(open('./tickets-create-configs.json'));
const profileName = readProfileName();
const profile = testConfigs[profileName];
const loadTestToken = __ENV.LOAD_TEST_METRICS_TOKEN;

export const options = {
  scenarios: {
    [`tickets_create_${profileName}`]: profile.scenario,
  },
  ...(Object.keys(profile.thresholds).length === 0
    ? {}
    : { thresholds: profile.thresholds }),
};

export function setup() {
  const password = 'performance-ticket-password';
  const email = `tickets-create-${runSuffix().toLowerCase()}@example.com`;
  const name = 'Ticket Create Performance User';

  assertSuccessfulResponse(
    http.post(
      `${baseUrl}/api/auth/signup`,
      JSON.stringify({ email, name, password }),
      requestParameters('tickets_create_auth_setup'),
    ),
    201,
    'sign up the performance user',
  );

  const verificationEmail = waitForVerificationEmail(email);
  const verificationToken = verificationTokenFrom(verificationEmail);
  assertSuccessfulResponse(
    http.get(
      `${baseUrl}/api/auth/verify-email?token=${encodeURIComponent(verificationToken)}`,
      requestParameters('tickets_create_auth_setup'),
    ),
    200,
    'verify the performance user email',
  );

  const signInResponse = http.post(
    `${baseUrl}/api/auth/signin`,
    JSON.stringify({ email, password }),
    requestParameters('tickets_create_auth_setup'),
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

export default function (performanceUser) {
  const title = `Performance ticket ${runSuffix()}-${__VU}-${__ITER}`;
  const price = 10_000;
  const response = http.post(
    `${baseUrl}/api/tickets`,
    JSON.stringify({ price, title }),
    {
      headers: {
        Cookie: performanceUser.sessionCookie,
        'Content-Type': 'application/json',
        'X-Ticketing-Load-Test-Token': loadTestToken,
      },
      tags: { endpoint: 'tickets_create' },
    },
  );

  check(
    response,
    {
      'returns HTTP 201': (result) => result.status === 201,
      'returns the created ticket': (result) =>
        hasCreatedTicket(result, title, price, performanceUser.userId),
    },
    { endpoint: 'tickets_create' },
  );

  if (profile.thinkTimeSeconds > 0) {
    sleep(profile.thinkTimeSeconds);
  }
}

function assertSuccessfulResponse(response, expectedStatus, action) {
  if (response.status !== expectedStatus) {
    throw new Error(
      `Unable to ${action}: expected HTTP ${expectedStatus}, received ${response.status}.`,
    );
  }
}

function hasCreatedTicket(
  response,
  expectedTitle,
  expectedPrice,
  expectedUserId,
) {
  if (response.status !== 201) {
    return false;
  }

  try {
    const body = response.json();
    return (
      typeof body?.id === 'string' &&
      body.id.length > 0 &&
      body.title === expectedTitle &&
      body.price === expectedPrice &&
      body.userId === expectedUserId
    );
  } catch {
    return false;
  }
}

function readProfileName() {
  const value = __ENV.K6_PROFILE || 'smoke';
  if (!Object.prototype.hasOwnProperty.call(testConfigs, value)) {
    throw new Error(
      `K6_PROFILE must be one of: ${Object.keys(testConfigs).join(', ')}`,
    );
  }

  return value;
}

function requestParameters(endpoint) {
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

function verificationTokenFrom(emailBody) {
  const match = emailBody.match(/\/api\/auth\/verify-email\?token=([^\s<"]+)/);
  if (!match) {
    throw new Error('Unable to find the email-verification token in Mailpit.');
  }

  return decodeURIComponent(match[1]);
}

function waitForVerificationEmail(recipient) {
  const timeoutAt = Date.now() + 10_000;
  while (Date.now() < timeoutAt) {
    const messagesResponse = http.get(
      `${mailpitUrl}/api/v1/messages`,
      requestParameters('tickets_create_auth_setup'),
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
        requestParameters('tickets_create_auth_setup'),
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
