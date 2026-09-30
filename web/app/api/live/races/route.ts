import { liveAPI } from '@/lib/f1api';

export async function GET(req: Request) {
  return liveAPI('/api/live/races', { signal: req.signal });
}
