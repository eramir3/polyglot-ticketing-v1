import http from 'k6/http';
import { check, sleep } from 'k6';

const baseUrl = __ENV.K6_BASE_URL || 'http://api-gateway:3000';
const expectedTicketCount = readExpectedTicketCount();

export const options = {
  scenarios: {
    tickets_list: {
      executor: 'ramping-vus',
      stages: [
        { duration: '10s', target: 1 },
        { duration: '30s', target: 5 },
        { duration: '10s', target: 0 },
      ],
    },
  },
  thresholds: {
    checks: ['rate==1'],
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1000'],
  },
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
  const value = __ENV.K6_EXPECT_TICKET_COUNT || '0';
  const count = Number(value);
  if (!Number.isSafeInteger(count) || count < 0) {
    throw new Error('K6_EXPECT_TICKET_COUNT must be a non-negative integer');
  }

  return count;
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

  sleep(1);
}
