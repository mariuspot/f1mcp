import { fetchImage } from '@/lib/f1api';

// Serves an image a tool returned, from the Go service. Images are named by
// their content's hash, so they can be cached for good.
export async function GET(req: Request, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  const resp = await fetchImage(id, req.signal);
  if (!resp.ok) {
    return new Response('Not found', { status: 404 });
  }
  return new Response(resp.body, {
    headers: {
      'Content-Type': resp.headers.get('Content-Type') ?? 'application/octet-stream',
      'Cache-Control': 'public, max-age=31536000, immutable',
    },
  });
}
