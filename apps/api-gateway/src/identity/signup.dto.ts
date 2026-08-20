import {
  IsEmail,
  IsNotEmpty,
  IsString,
  Length,
  Matches,
} from 'class-validator';

export class SignUpDto {
  @IsString({ message: 'Name is required.' })
  @IsNotEmpty({ message: 'Name is required.' })
  @Matches(/\S/, { message: 'Name is required.' })
  name!: string;

  @IsEmail({}, { message: 'Email must be valid.' })
  email!: string;

  @IsString({ message: 'Password must be between 8 and 128 characters.' })
  @Length(8, 128, {
    message: 'Password must be between 8 and 128 characters.',
  })
  password!: string;
}
