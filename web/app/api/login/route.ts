import { NextResponse } from 'next/server';
import { equal, passcode, sessionCookie, sessionToken } from '@/lib/auth';
import { allowSignIn } from '@/lib/limits';

// origin is the address the visitor used. Behind Caddy, req.url is the
// container's own address, so use the forwarded host and protocol.
function origin(req: Request) {
  const url = new URL(req.url);
  const host = req.headers.get('x-forwarded-host') ?? req.headers.get('host') ?? url.host;
  const proto = req.headers.get('x-forwarded-proto') ?? url.protocol.replace(':', '');
  return `${proto}://${host}`;
}

// Signs in with the passcode from the sign-in form, then goes to the chat.
export async function POST(req: Request) {
  const back = (error: string) => NextResponse.redirect(new URL(`/login?error=${error}`, origin(req)), 303);
  if (!allowSignIn(req)) return back('slow');
  const form = await req.formData();
  const given = String(form.get('passcode') ?? '');
  if (!passcode() || !equal(given, passcode())) return back('wrong');

  const res = NextResponse.redirect(new URL('/', origin(req)), 303);
  res.cookies.set(sessionCookie, await sessionToken(), {
    httpOnly: true,
    secure: origin(req).startsWith('https:'),
    sameSite: 'lax',
    path: '/',
    maxAge: 90 * 24 * 60 * 60,
  });
  return res;
}
