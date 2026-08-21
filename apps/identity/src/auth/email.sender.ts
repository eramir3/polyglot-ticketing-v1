import { createTransport } from 'nodemailer';

export interface EmailSender {
  sendVerificationEmail(input: {
    recipient: string;
    verificationUrl: string;
  }): Promise<void>;
}

export function createEmailSender(): EmailSender {
  const transporter = createTransport({
    host: requiredEnvironmentVariable('SMTP_HOST'),
    port: Number(requiredEnvironmentVariable('SMTP_PORT')),
    secure: false,
  });
  const from = requiredEnvironmentVariable('SMTP_FROM');

  return {
    async sendVerificationEmail({ recipient, verificationUrl }): Promise<void> {
      await transporter.sendMail({
        from,
        subject: 'Verify your Polyglot Ticketing email address',
        text: `Verify your email address: ${verificationUrl}`,
        to: recipient,
      });
    },
  };
}

export function createVerificationUrl(token: string): string {
  const url = new URL(requiredEnvironmentVariable('EMAIL_VERIFICATION_URL'));
  url.searchParams.set('token', token);
  return url.toString();
}

function requiredEnvironmentVariable(name: string): string {
  const value = process.env[name];

  if (!value) {
    throw new Error(`${name} must be configured.`);
  }

  return value;
}
