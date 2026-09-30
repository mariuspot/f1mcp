// Calls the f1mcp Go service's web API, which runs the same tools as its MCP
// server and returns their images as links.

export type ImageLink = {
  name: string; // what it shows, e.g. "map", "corner", "faster"
  url: string; // e.g. "/img/3f2a….png", served through this app
  mime_type: string;
};

export type ToolOutput<T = unknown> = {
  result: T;
  images?: ImageLink[];
};

const apiURL = () => process.env.F1_API_URL ?? 'http://localhost:8080';

// callTool runs a tool on the Go service. A tool's own errors, such as "no
// red flag in …", are thrown with its message so the model can act on them.
export async function callTool<T>(
  name: string,
  args: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<ToolOutput<T>> {
  // Leave out empty arguments rather than sending zero values.
  const body = Object.fromEntries(
    Object.entries(args).filter(([, v]) => v !== undefined && v !== null && v !== ''),
  );
  const resp = await fetch(`${apiURL()}/api/tools/${name}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  const data = await resp.json().catch(() => ({ error: `${resp.status} ${resp.statusText}` }));
  if (!resp.ok) {
    throw new Error(data.error ?? `${name} failed with ${resp.status}`);
  }
  return data as ToolOutput<T>;
}

// fetchImage gets an image a tool returned, for the /img route.
export function fetchImage(id: string, signal?: AbortSignal) {
  return fetch(`${apiURL()}/img/${encodeURIComponent(id)}`, { signal });
}

// liveAPI passes a live page request through to the Go service.
export async function liveAPI(path: string, init: RequestInit = {}) {
  const resp = await fetch(`${apiURL()}${path}`, { ...init, cache: 'no-store' });
  return new Response(resp.body, { status: resp.status, headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' } });
}
