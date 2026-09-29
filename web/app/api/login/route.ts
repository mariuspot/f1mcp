import { NextResponse } from 'next/server';
import { equal, passcode, sessionCookie, sessionToken } from '@/lib/auth';
import { allowSignIn } from '@/lib/limits';

// Signs in with the passcode from the sign-in form, then goes to the chat.
export async function POST(req: Request) {
  const back = (error: string) => NextResponse.redirect(new URL(`/login?error=${error}`, req.url), 303);
  if (!allowSignIn(req)) return back('slow');
  const form = await req.formData();
  const given = String(form.get('passcode') ?? '');
  if (!passcode() || !equal(given, passcode())) return back('wrong');

  const res = NextResponse.redirect(new URL('/', req.url), 303);
  res.cookies.set(sessionCookie, await sessionToken(), {
    httpOnly: true,
    secure: new URL(req.url).protocol === 'https:' || req.headers.get('x-forwarded-proto') === 'https',
    sameSite: 'lax',
    path: '/',
    maxAge: 90 * 24 * 60 * 60,
  });
  return res;
}
