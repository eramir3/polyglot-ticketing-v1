import { IsEmail, IsString, Length } from 'class-validator';

export class SignInDto {
  @IsEmail({}, { message: 'Email must be valid' })
  email!: string;

  @IsString({ message: 'Password must be a string' })
  @Length(8, 128, { message: 'Password must be between 8 and 128 characters' })
  password!: string;
}
