import { createValidator } from '@bufbuild/protovalidate';
import { create } from '@bufbuild/protobuf';
import { SignUpRequestSchema } from '../../../../../protogen/ts/identity/v1/identity_pb.js';
import { ErrorItem } from '../../errors/grpc-error';
import { SignUpRequest } from '../identity.types';
import { mapProtovalidateViolation } from '../mappers/protovalidate-violation.mapper';

type SignUpValidationResult =
  | { kind: 'valid' }
  | { kind: 'invalid'; errors: ErrorItem[] }
  | { kind: 'error' };

const validator = createValidator();

export function validateSignUpRequest(
  request: SignUpRequest,
): SignUpValidationResult {
  const result = validator.validate(
    SignUpRequestSchema,
    create(SignUpRequestSchema, request),
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
