export const metadata = { title: 'Sign in · Pit Wall' };

const messages: Record<string, string> = {
  wrong: 'That passcode isn’t right.',
  slow: 'Too many tries. Wait a minute and try again.',
};

export default async function Login({ searchParams }: PageProps<'/login'>) {
  const { error } = await searchParams;
  const message = typeof error === 'string' ? messages[error] : undefined;
  return (
    <main className="flex min-h-dvh items-center justify-center px-4">
      <form method="post" action="/api/login" className="w-full max-w-sm rounded-xl border border-line bg-panel p-6">
        <div className="flex items-center gap-3">
          <div className="h-5 w-1.5 rounded-sm bg-accent" />
          <h1 className="text-lg font-semibold">Pit Wall</h1>
        </div>
        <p className="mt-2 text-sm text-muted">Enter the passcode to ask about Formula 1.</p>
        <input
          name="passcode"
          type="password"
          autoComplete="current-password"
          autoFocus
          required
          placeholder="Passcode"
          className="mt-4 h-11 w-full rounded-lg border border-line bg-bg px-3 outline-none focus:border-accent/60"
        />
        {message && <p className="mt-2 text-sm text-accent">{message}</p>}
        <button type="submit" className="mt-4 h-11 w-full rounded-lg bg-accent font-medium text-white">
          Sign in
        </button>
      </form>
    </main>
  );
}
