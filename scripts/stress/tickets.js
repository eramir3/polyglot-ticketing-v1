'use strict';

const { randomUUID } = require('node:crypto');

const DEFAULT_BASE_URL = 'http://localhost:3000';
const DEFAULT_LIMIT = 200;
const INITIAL_PRICE = 5;
const FIRST_UPDATE_PRICE = 10;
const FINAL_UPDATE_PRICE = 15;
const PROGRESS_INTERVAL = 25;

class StressRequestError extends Error {
  constructor(message, status, body) {
    super(message);
    this.name = 'StressRequestError';
    this.status = status;
    this.body = body;
  }
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

function findStressRequestError(error) {
  let current = error;
  while (current instanceof Error) {
    if (current instanceof StressRequestError) {
      return current;
    }
    current = current.cause;
  }
  return undefined;
}

function readPositiveInteger(value, fallback) {
  if (value === undefined || value.trim() === '') {
    return fallback;
  }

  const parsedValue = Number(value);
  if (!Number.isSafeInteger(parsedValue) || parsedValue <= 0) {
    throw new Error('STRESS_LIMIT must be a positive integer');
  }

  return parsedValue;
}

function readConfig() {
  //const cookie = process.env.STRESS_COOKIE;
  const cookie = 'better-auth.session_token=bzdz3AQKnZhcRUQI3wbAtdHg1bArV1N5.gbiMV9eRFvIZo8T58oZkv%2B7b8jmQgZu09SiJoRK6FWM%3D'
  if (!cookie) {
    throw new Error('STRESS_COOKIE is required');
  }

  const baseUrl = process.env.STRESS_BASE_URL ?? DEFAULT_BASE_URL;
  let ticketsUrl;
  try {
    ticketsUrl = new URL('/api/tickets', baseUrl).toString().replace(/\/$/, '');
  } catch {
    throw new Error('STRESS_BASE_URL must be a valid URL');
  }

  return {
    cookie,
    insecureTls: process.env.STRESS_INSECURE_TLS === '1',
    limit: readPositiveInteger(process.env.STRESS_LIMIT, DEFAULT_LIMIT),
    ticketsUrl,
  };
}

async function requestJson(config, url, options = {}) {
  let response;
  try {
    response = await fetch(url, {
      ...options,
      headers: {
        accept: 'application/json',
        cookie: config.cookie,
        ...options.headers,
      },
    });
  } catch (error) {
    throw new Error(`Request to ${url} failed: ${errorMessage(error)}`, {
      cause: error,
    });
  }

  const rawBody = await response.text();
  let body;
  try {
    body = rawBody === '' ? undefined : JSON.parse(rawBody);
  } catch {
    body = rawBody;
  }

  if (!response.ok) {
    throw new StressRequestError(
      `Request to ${url} returned ${response.status}`,
      response.status,
      body,
    );
  }

  return { body, status: response.status };
}

function assertStatus(response, expectedStatus, action) {
  if (response.status !== expectedStatus) {
    throw new Error(
      `${action} returned ${response.status}; expected ${expectedStatus}`,
    );
  }
}

function assertTicket(ticket, expected, action) {
  if (
    ticket === null ||
    typeof ticket !== 'object' ||
    ticket.id !== expected.id ||
    ticket.price !== expected.price ||
    ticket.title !== expected.title
  ) {
    throw new Error(
      `${action} returned an unexpected ticket: ${JSON.stringify(ticket)}`,
    );
  }
}

function ticketUrl(ticketsUrl, ticketId) {
  return `${ticketsUrl}/${encodeURIComponent(ticketId)}`;
}

async function createTicket(config, title) {
  const response = await requestJson(config, config.ticketsUrl, {
    body: JSON.stringify({ price: INITIAL_PRICE, title }),
    headers: { 'content-type': 'application/json' },
    method: 'POST',
  });
  assertStatus(response, 201, 'Ticket creation');

  if (
    response.body === null ||
    typeof response.body !== 'object' ||
    typeof response.body.id !== 'string' ||
    response.body.id === ''
  ) {
    throw new Error(
      `Ticket creation returned an unexpected response: ${JSON.stringify(response.body)}`,
    );
  }

  return response.body.id;
}

async function updateTicket(config, ticketId, title, price, action) {
  const response = await requestJson(
    config,
    ticketUrl(config.ticketsUrl, ticketId),
    {
      body: JSON.stringify({ price, title }),
      headers: { 'content-type': 'application/json' },
      method: 'PUT',
    },
  );
  assertStatus(response, 200, action);
  assertTicket(response.body, { id: ticketId, price, title }, action);
}

async function runCycle(config, iteration, runId) {
  const baseTitle = `stress-ticket-${runId}-${iteration + 1}`;
  const ticketId = await createTicket(config, baseTitle);
  const firstUpdateTitle = `${baseTitle}-first-update`;
  const finalTitle = `${baseTitle}-second-update`;

  await updateTicket(
    config,
    ticketId,
    firstUpdateTitle,
    FIRST_UPDATE_PRICE,
    'First ticket update',
  );
  await updateTicket(
    config,
    ticketId,
    finalTitle,
    FINAL_UPDATE_PRICE,
    'Second ticket update',
  );

  return { id: ticketId, price: FINAL_UPDATE_PRICE, title: finalTitle };
}

async function verifyTicket(config, ticket) {
  const response = await requestJson(
    config,
    ticketUrl(config.ticketsUrl, ticket.id),
  );
  assertStatus(response, 200, 'Final ticket retrieval');
  assertTicket(response.body, ticket, 'Final ticket retrieval');
}

async function main() {
  const config = readConfig();
  if (config.insecureTls) {
    process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';
    console.warn('TLS certificate verification is disabled for this run.');
  }

  console.log(
    `Running ${config.limit} sequential ticket create/update cycles against ${config.ticketsUrl}`,
  );

  const runId = randomUUID();
  const finalTickets = [];
  for (let index = 0; index < config.limit; index += 1) {
    try {
      finalTickets.push(await runCycle(config, index, runId));
    } catch (error) {
      throw new Error(`Cycle ${index + 1} failed: ${errorMessage(error)}`, {
        cause: error,
      });
    }

    if ((index + 1) % PROGRESS_INTERVAL === 0 || index === config.limit - 1) {
      console.log(`Completed ${index + 1}/${config.limit} ticket cycles`);
    }
  }

  console.log(`Verifying ${finalTickets.length} final ticket states`);
  for (let index = 0; index < finalTickets.length; index += 1) {
    try {
      await verifyTicket(config, finalTickets[index]);
    } catch (error) {
      throw new Error(
        `Verification ${index + 1} failed: ${errorMessage(error)}`,
        {
          cause: error,
        },
      );
    }

    if (
      (index + 1) % PROGRESS_INTERVAL === 0 ||
      index === finalTickets.length - 1
    ) {
      console.log(`Verified ${index + 1}/${finalTickets.length} tickets`);
    }
  }
}

main()
  .then(() => {
    console.log('Ticket stress run complete');
  })
  .catch((error) => {
    console.error('Ticket stress run failed');
    console.error(errorMessage(error));
    const requestError = findStressRequestError(error);
    if (requestError) {
      console.error('Status:', requestError.status);
      console.error('Response:', JSON.stringify(requestError.body));
    }
    process.exitCode = 1;
  });
