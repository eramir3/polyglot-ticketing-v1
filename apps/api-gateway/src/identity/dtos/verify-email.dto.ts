import { IsNotEmpty } from 'class-validator';

export class VerifyEmailDto {
  @IsNotEmpty({ message: 'You must supply a verification token' })
  token!: string;
}
