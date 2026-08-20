import { HttpException, HttpStatus } from '@nestjs/common';

export interface ErrorItem {
  message: string;
  code: string;
  field?: string;
}

export interface ErrorResponse {
  errors: ErrorItem[];
}

export class ApiError extends HttpException {
  constructor(status: HttpStatus, errors: ErrorItem[]) {
    super(
      {
        errors: errors.map((error) => ({
          message: error.message,
          code: error.code,
          ...(error.field === undefined ? {} : { field: error.field }),
        })),
      } satisfies ErrorResponse,
      status,
    );
  }
}
