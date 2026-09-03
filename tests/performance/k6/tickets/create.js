import http from 'k6/http';
import { check, sleep } from 'k6';
import { createVerifiedPerformanceUser } from '../shared/auth.js';
import { metricSystemTags } from '../shared/metrics.js';

const baseUrl = __ENV.K6_BASE_URL || 'http://api-gateway:3000';
const mailpitUrl = __ENV.K6_MAILPIT_URL || 'http://mailpit:8025';
const testConfigs = JSON.parse(open('./create-configs.json'));
const profileName = readProfileName();
const profile = testConfigs[profileName];
const loadTestToken = __ENV.LOAD_TEST_METRICS_TOKEN;

export const options = {
  systemTags: metricSystemTags,
  scenarios: {
    [`tickets_create_${profileName}`]: profile.scenario,
  },
  ...(Object.keys(profile.thresholds).length === 0
    ? {}
    : { thresholds: profile.thresholds }),
};

export function setup() {
  return createVerifiedPerformanceUser({
    baseUrl,
    loadTestToken,
    mailpitUrl,
    setupEndpoint: 'tickets_create_auth_setup',
    userName: 'Ticket Create Performance User',
    userPrefix: 'tickets-create',
  });
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
      tags: { endpoint: 'tickets_create', name: 'tickets_create' },
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

function runSuffix() {
  const runId = String(__ENV.K6_TEST_ID || Date.now()).replace(
    /[^a-zA-Z0-9]/g,
    '-',
  );
  return `${runId}-${Math.floor(Math.random() * 1_000_000)}`;
}
