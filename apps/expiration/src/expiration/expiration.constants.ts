export const expirationQueueName = 'expiration';
export const expirationJobName = 'expiration-complete';
export const expirationEventsStream = 'EXPIRATION_EVENTS';
export const expirationDeadLetterStream = 'EXPIRATION_DLQ';
export const expirationCompleteSubject = 'expiration.expiration.complete.v1';
export const ordersEventsStream = 'ORDERS_EVENTS';
export const orderCreatedSubject = 'orders.order.created.v1';
export const orderCreatedDurableName = 'expiration-order-created-v1';
export const orderCreatedDeadLetterSubject = 'dlq.expiration.order-created.v1';
export const orderCreatedMaxRetries = 5;
export const expirationPublishAttempts = 5;
export const expirationPublishBackoffMilliseconds = 1_000;
export const natsRetryMilliseconds = 1_000;

export const EXPIRATION_EVENT_PUBLISHER = Symbol('EXPIRATION_EVENT_PUBLISHER');
