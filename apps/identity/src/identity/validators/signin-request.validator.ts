import { createValidator } from '@bufbuild/protovalidate';
import { create } from '@bufbuild/protobuf';
import { SignInRequestSchema } from '../../../../../protogen/ts/identity/v1/identity_pb.js';
import { ErrorItem } from '../../errors/grpc-error';
import { SignInRequest } from '../identity.types';
import { mapProtovalidateViolation } from '../mappers/protovalidate-violation.mapper';

type SignInValidationResult =
  | { kind: 'valid' }
  | { kind: 'invalid'; errors: ErrorItem[] }
  | { kind: 'error' };

const validator = createValidator();

export function validateSignInRequest(
  request: SignInRequest,
): SignInValidationResult {
  const result = validator.validate(
    SignInRequestSchema,
    create(SignInRequestSchema, request),
  );

  if (result.kind === 'valid') {
    return { kind: 'valid' };
  }

  if (result.kind === 'invalid') {
    return {
      kind: 'invalid',
      errors: result.violations.map(mapProtovalidateViolation),
    };
  }

  return { kind: 'error' };
}
