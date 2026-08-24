import { IncomingHttpHeaders } from 'node:http';
import {
  Body,
  Controller,
  Get,
  Headers,
  HttpCode,
  HttpStatus,
  Post,
  Query,
  Res,
} from '@nestjs/common';
import {
  CurrentUserResponse,
  SignInResponse,
  SignUpResponse,
  VerifyEmailResponse,
} from './identity.types';
import { AuthService } from './auth.service';
import { SignInDto } from './dtos/signin.dto';
import { SignUpDto } from './dtos/signup.dto';
import { VerifyEmailDto } from './dtos/verify-email.dto';
import { SESSION_COOKIE_NAME } from './identity.constants';

@Controller('auth')
export class AuthController {
  constructor(private readonly authService: AuthService) {}

  @Post('signup')
  signUp(@Body() dto: SignUpDto): Promise<SignUpResponse> {
    return this.authService.signUp(dto);
  }

  @Post('signin')
  async signIn(
    @Body() dto: SignInDto,
    @Headers() headers: IncomingHttpHeaders,
    @Res({ passthrough: true }) response: CookieResponse,
  ): Promise<SignInResponse> {
    const result = await this.authService.signIn(dto, headers);
    const expiresAt = toDate(result.sessionExpiresAt);
    response.cookie(SESSION_COOKIE_NAME, result.sessionToken, {
      encode: (value) => value,
      expires: expiresAt,
      httpOnly: true,
      path: '/',
      sameSite: 'lax',
      secure: process.env.NODE_ENV === 'production',
    });

    return {
      session: { expiresAt: expiresAt.toISOString() },
      user: {
        email: result.email,
        emailVerified: result.emailVerified,
        id: result.userId,
        name: result.name,
      },
    };
  }

  @Post('signout')
  @HttpCode(HttpStatus.NO_CONTENT)
  async signOut(
    @Headers() headers: IncomingHttpHeaders,
    @Res({ passthrough: true }) response: CookieResponse,
  ): Promise<void> {
    await this.authService.signOut(headers);
    response.cookie(SESSION_COOKIE_NAME, '', {
      encode: (value) => value,
      expires: new Date(0),
      httpOnly: true,
      path: '/',
      sameSite: 'lax',
      secure: process.env.NODE_ENV === 'production',
    });
  }

  @Get('currentuser')
  currentUser(
    @Headers() headers: IncomingHttpHeaders,
  ): Promise<CurrentUserResponse> {
    return this.authService.currentUser(headers);
  }

  @Get('verify-email')
  verifyEmail(@Query() dto: VerifyEmailDto): Promise<VerifyEmailResponse> {
    return this.authService.verifyEmail(dto);
  }
}

function toDate(timestamp: {
  seconds: string | number | bigint;
  nanos: number;
}): Date {
  return new Date(
    Number(timestamp.seconds) * 1_000 + timestamp.nanos / 1_000_000,
  );
}

interface CookieResponse {
  cookie(
    name: string,
    value: string,
    options: {
      encode: (value: string) => string;
      expires: Date;
      httpOnly: boolean;
      path: string;
      sameSite: 'lax';
      secure: boolean;
    },
  ): void;
}
