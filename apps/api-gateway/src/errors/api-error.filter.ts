import {
  ArgumentsHost,
  Catch,
  ExceptionFilter,
  HttpException,
  HttpStatus,
} from '@nestjs/common';
import { ErrorItem, ErrorResponse } from './api-error';

interface HttpResponse {
  status(statusCode: number): { json(body: ErrorResponse): void };
}

@Catch()
export class ApiErrorFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost): void {
    const response = host.switchToHttp().getResponse<HttpResponse>();
    const status = getHttpStatus(exception);
    response.status(status).json(getErrorResponse(exception, status));
  }
}

function getHttpStatus(exception: unknown): number {
  if (exception instanceof HttpException) {
    return exception.getStatus();
  }

  return HttpStatus.INTERNAL_SERVER_ERROR;
}

function getErrorResponse(exception: unknown, status: number): ErrorResponse {
  if (exception instanceof HttpException && status < 500) {
    const response = exception.getResponse();
    const structuredResponse = getStructuredResponse(response);
    if (structuredResponse) {
      return structuredResponse;
    }

    return {
      errors: getMessages(response).map((message) => ({
        message,
        code: getCodeForStatus(status),
      })),
    };
  }

  return {
    errors: [
      {
        message:
          status === HttpStatus.SERVICE_UNAVAILABLE ||
          status === HttpStatus.BAD_GATEWAY
            ? 'A required service is unavailable.'
            : 'An unexpected error occurred.',
        code:
          status === HttpStatus.SERVICE_UNAVAILABLE ||
          status === HttpStatus.BAD_GATEWAY
            ? 'SERVICE_UNAVAILABLE'
            : 'INTERNAL_ERROR',
      },
    ],
  };
}

function getStructuredResponse(
  response: string | object,
): ErrorResponse | undefined {
  if (
    typeof response === 'object' &&
    response !== null &&
    'errors' in response &&
    Array.isArray(response.errors) &&
    response.errors.length > 0 &&
    response.errors.every(isErrorItem)
  ) {
    return { errors: response.errors };
  }

  return undefined;
}

function getMessages(response: string | object): string[] {
  if (typeof response === 'string') {
    return [response];
  }

  if (response !== null && 'message' in response) {
    if (Array.isArray(response.message)) {
      return response.message.filter(
        (message): message is string => typeof message === 'string',
      );
    }

    if (typeof response.message === 'string') {
      return [response.message];
    }
  }

  return ['Request failed.'];
}

function getCodeForStatus(status: number): string {
  switch (status) {
    case HttpStatus.BAD_REQUEST:
      return 'INVALID_ARGUMENT';
    case HttpStatus.UNAUTHORIZED:
      return 'UNAUTHENTICATED';
    case HttpStatus.FORBIDDEN:
      return 'FORBIDDEN';
    case HttpStatus.NOT_FOUND:
      return 'NOT_FOUND';
    case HttpStatus.CONFLICT:
      return 'ALREADY_EXISTS';
    case HttpStatus.TOO_MANY_REQUESTS:
      return 'RATE_LIMITED';
    default:
      return 'INTERNAL_ERROR';
  }
}

function isErrorItem(value: unknown): value is ErrorItem {
  return (
    typeof value === 'object' &&
    value !== null &&
    'message' in value &&
    typeof value.message === 'string' &&
    'code' in value &&
    typeof value.code === 'string' &&
    (!('field' in value) || typeof value.field === 'string')
  );
}
