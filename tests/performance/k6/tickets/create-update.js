import http from 'k6/http';
import { check, sleep } from 'k6';
import { createVerifiedPerformanceUser } from '../shared/auth.js';
import { metricSystemTags } from '../shared/metrics.js';

const baseUrl = __ENV.K6_BASE_URL || 'http://api-gateway:3000';
const mailpitUrl = __ENV.K6_MAILPIT_URL || 'http://mailpit:8025';
const testConfigs = JSON.parse(open('./create-update-configs.json'));
const profileName = readProfileName();
const profile = testConfigs[profileName];
const loadTestToken = __ENV.LOAD_TEST_METRICS_TOKEN;

const initialPrice = 5;
const firstUpdatePrice = 10;
const finalUpdatePrice = 15;

export const options = {
  systemTags: metricSystemTags,
  scenarios: {
    [`tickets_create_update_${profileName}`]: profile.scenario,
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
    setupEndpoint: 'tickets_create_update_auth_setup',
    userName: 'Ticket Create and Update Performance User',
    userPrefix: 'tickets-create-update',
  });
}

export default function (performanceUser) {
  runLifecycle(performanceUser);

  if (profile.thinkTimeSeconds > 0) {
    sleep(profile.thinkTimeSeconds);
  }
}

function runLifecycle(performanceUser) {
  const baseTitle = `Performance ticket lifecycle ${runSuffix()}-${__VU}-${__ITER}`;
  const created = createTicket(performanceUser, baseTitle);
  if (!created) {
    return;
  }

  const firstUpdate = {
    id: created.id,
    price: firstUpdatePrice,
    title: `${baseTitle}-first-update`,
    userId: performanceUser.userId,
  };
  if (!updateTicket(performanceUser, firstUpdate, 'tickets_create_update_first_update')) {
    return;
  }

  const finalTicket = {
    ...firstUpdate,
    price: finalUpdatePrice,
    title: `${baseTitle}-second-update`,
  };
  if (!updateTicket(performanceUser, finalTicket, 'tickets_create_update_final_update')) {
    return;
  }

  const response = http.get(
    ticketUrl(finalTicket.id),
    requestParameters(performanceUser, 'tickets_create_update_get'),
  );
  check(
    response,
    {
      'final retrieval returns HTTP 200': (result) => result.status === 200,
      'final retrieval returns the final ticket state': (result) =>
        hasTicket(result, finalTicket, 200),
    },
    { endpoint: 'tickets_create_update_get' },
  );
}

function createTicket(performanceUser, title) {
  const expectedTicket = {
    price: initialPrice,
    title,
    userId: performanceUser.userId,
  };
  const response = http.post(
    `${baseUrl}/api/tickets`,
    JSON.stringify({ price: expectedTicket.price, title: expectedTicket.title }),
    requestParameters(performanceUser, 'tickets_create_update_create'),
  );
  const succeeded = check(
    response,
    {
      'creation returns HTTP 201': (result) => result.status === 201,
      'creation returns the created ticket': (result) =>
        hasTicket(result, expectedTicket, 201),
    },
    { endpoint: 'tickets_create_update_create' },
  );
  if (!succeeded) {
    return undefined;
  }

  return response.json();
}

function updateTicket(performanceUser, expectedTicket, endpoint) {
  const response = http.put(
    ticketUrl(expectedTicket.id),
    JSON.stringify({ price: expectedTicket.price, title: expectedTicket.title }),
    requestParameters(performanceUser, endpoint),
  );
  return check(
    response,
    {
      'update returns HTTP 200': (result) => result.status === 200,
      'update returns the expected ticket': (result) =>
        hasTicket(result, expectedTicket, 200),
    },
    { endpoint },
  );
}

function hasTicket(response, expectedTicket, expectedStatus) {
  if (response.status !== expectedStatus) {
    return false;
  }

  try {
    const body = response.json();
    return (
      typeof body?.id === 'string' &&
      body.id.length > 0 &&
      (expectedTicket.id === undefined || body.id === expectedTicket.id) &&
      body.title === expectedTicket.title &&
      body.price === expectedTicket.price &&
      body.userId === expectedTicket.userId
    );
  } catch {
    return false;
  }
}

function requestParameters(performanceUser, endpoint) {
  return {
    headers: {
      Cookie: performanceUser.sessionCookie,
      'Content-Type': 'application/json',
      'X-Ticketing-Load-Test-Token': loadTestToken,
    },
    tags: { endpoint, name: endpoint },
  };
}

function ticketUrl(ticketId) {
  return `${baseUrl}/api/tickets/${encodeURIComponent(ticketId)}`;
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
