import { liveAPI } from '@/lib/f1api';

// Starts or stops the replay shown on the live page.
export async function POST(req: Request) {
  return liveAPI('/api/live/replay', { method: 'POST', body: await req.text(), headers: { 'Content-Type': 'application/json' }, signal: req.signal });
}
