import http from 'k6/http';
import { check, sleep } from 'k6';

const baseUrl = __ENV.K6_BASE_URL || 'http://api-gateway:3000';
const testConfigs = JSON.parse(open('./tickets-list-configs.json'));
const profileName = readProfileName();
const profile = testConfigs[profileName];
const expectedTicketCount = readExpectedTicketCount();

export const options = {
  scenarios: {
    [`tickets_list_${profileName}`]: profile.scenario,
  },
  ...(Object.keys(profile.thresholds).length === 0
    ? {}
    : { thresholds: profile.thresholds }),
};

function hasJsonArrayBody(response) {
  try {
    return Array.isArray(response.json());
  } catch {
    return false;
  }
}

function hasExpectedTicketCount(response) {
  if (!hasJsonArrayBody(response)) {
    return false;
  }

  return response.json().length === expectedTicketCount;
}

function readExpectedTicketCount() {
  const value = __ENV.K6_EXPECT_TICKET_COUNT || '100';
  const count = Number(value);
  if (!Number.isSafeInteger(count) || count < 0) {
    throw new Error('K6_EXPECT_TICKET_COUNT must be a non-negative integer');
  }

  return count;
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

export default function () {
  const response = http.get(`${baseUrl}/api/tickets`, {
    headers: {
      'X-Ticketing-Load-Test-Token': __ENV.LOAD_TEST_METRICS_TOKEN,
    },
    tags: { endpoint: 'tickets_list' },
  });

  check(response, {
    'returns HTTP 200': (result) => result.status === 200,
    'returns a JSON array': hasJsonArrayBody,
    'returns the expected ticket count': hasExpectedTicketCount,
  });

  if (profile.thinkTimeSeconds > 0) {
    sleep(profile.thinkTimeSeconds);
  }
}
