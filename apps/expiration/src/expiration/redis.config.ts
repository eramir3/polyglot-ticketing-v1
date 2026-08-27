import type { ConnectionOptions } from 'bullmq';

export function redisConnectionOptions(
  redisUrl = process.env.REDIS_URL ?? 'redis://localhost:6379',
): ConnectionOptions {
  const url = new URL(redisUrl);
  const database =
    url.pathname === '' || url.pathname === '/'
      ? 0
      : Number(url.pathname.slice(1));

  if (!Number.isInteger(database) || database < 0) {
    throw new Error(
      'REDIS_URL must contain a non-negative integer database index.',
    );
  }

  return {
    host: url.hostname,
    port: Number(url.port || 6379),
    username: url.username || undefined,
    password: url.password || undefined,
    db: database,
    maxRetriesPerRequest: null,
  };
}
