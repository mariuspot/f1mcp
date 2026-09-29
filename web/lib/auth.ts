// A shared passcode guards the app when PITWALL_PASSCODE is set (it isn't in
// local development). Signing in sets a cookie holding an HMAC of a fixed
// string keyed by the passcode, so changing the passcode signs everyone out.

export const sessionCookie = 'pitwall_session';

export function passcode() {
  return process.env.PITWALL_PASSCODE ?? '';
}

async function hmac(key: string, message: string) {
  const k = await crypto.subtle.importKey('raw', new TextEncoder().encode(key), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const sig = await crypto.subtle.sign('HMAC', k, new TextEncoder().encode(message));
  return Array.from(new Uint8Array(sig), b => b.toString(16).padStart(2, '0')).join('');
}

// sessionToken is the cookie value for the current passcode.
export function sessionToken() {
  return hmac(passcode(), 'pit-wall-session-v1');
}

// equal compares two strings in time independent of where they differ.
export function equal(a: string, b: string) {
  let diff = a.length ^ b.length;
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    diff |= (a.charCodeAt(i) || 0) ^ (b.charCodeAt(i) || 0);
  }
  return diff === 0;
}

export async function signedIn(cookie: string | undefined) {
  if (!passcode()) return true;
  return !!cookie && equal(cookie, await sessionToken());
}
