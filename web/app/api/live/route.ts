import { liveAPI } from '@/lib/f1api';

// The live page's state and new events, from the Go service.
export async function GET(req: Request) {
  const since = new URL(req.url).searchParams.get('since') ?? '0';
  return liveAPI(`/api/live?since=${encodeURIComponent(since)}`, { signal: req.signal });
}
