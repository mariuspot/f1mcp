// Limits on how much the app is used, so a leaked passcode can't run up a
// large bill: questions per visitor per hour, questions per day in all, and
// sign-in attempts per visitor per minute. Counts are kept in memory, which
// is enough for the one server this runs on.

type Window = { start: number; count: number };

const windows = new Map<string, Window>();

// take counts one use of key in a window of ms, and reports whether it is
// within max, and if not, how many seconds until the window resets.
function take(key: string, max: number, ms: number): { ok: true } | { ok: false; retryAfter: number } {
  const now = Date.now();
  let w = windows.get(key);
  if (!w || now - w.start >= ms) {
    w = { start: now, count: 0 };
    windows.set(key, w);
  }
  if (w.count >= max) {
    return { ok: false, retryAfter: Math.ceil((w.start + ms - now) / 1000) };
  }
  w.count++;
  return { ok: true };
}

const hour = 60 * 60 * 1000;
const envInt = (name: string, fallback: number) => Number.parseInt(process.env[name] ?? '', 10) || fallback;

// visitor identifies who is asking: the client's address, from Caddy's
// X-Forwarded-For in production.
export function visitor(req: Request) {
  return req.headers.get('x-forwarded-for')?.split(',')[0].trim() || 'local';
}

// allowQuestion checks the hourly and daily limits for a question, or
// returns a response to send instead.
export function allowQuestion(req: Request): Response | null {
  const daily = take('day', envInt('PITWALL_DAILY_LIMIT', 300), 24 * hour);
  if (!daily.ok) {
    return tooMany(`Pit Wall has answered all the questions it can for today. Try again in ${Math.ceil(daily.retryAfter / 3600)} hours.`, daily.retryAfter);
  }
  const hourly = take(`q:${visitor(req)}`, envInt('PITWALL_HOURLY_LIMIT', 30), hour);
  if (!hourly.ok) {
    return tooMany(`That's the limit of questions for this hour. Try again in ${Math.ceil(hourly.retryAfter / 60)} minutes.`, hourly.retryAfter);
  }
  return null;
}

export function allowSignIn(req: Request) {
  return take(`login:${visitor(req)}`, 5, 60 * 1000).ok;
}

function tooMany(message: string, retryAfter: number) {
  return new Response(message, { status: 429, headers: { 'Retry-After': String(retryAfter), 'Content-Type': 'text/plain' } });
}
