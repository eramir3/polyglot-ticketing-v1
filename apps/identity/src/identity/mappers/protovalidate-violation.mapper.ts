import { pathToString } from '@bufbuild/protobuf/reflect';
import { type Violation } from '@bufbuild/protovalidate';
import { ErrorItem } from '../../errors/grpc-error';
import {
  createInvalidArgumentError,
  createInvalidEmailError,
  createInvalidNameError,
  createInvalidPasswordError,
} from './signup-errors';

export function mapProtovalidateViolation(violation: Violation): ErrorItem {
  switch (pathToString(violation.field)) {
    case 'name':
      return createInvalidNameError();
    case 'email':
      return createInvalidEmailError();
    case 'password':
      return createInvalidPasswordError();
    default:
      return createInvalidArgumentError();
  }
}
