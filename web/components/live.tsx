'use client';

import Link from 'next/link';
import { useCallback, useEffect, useRef, useState } from 'react';
import type { Car, Flag, Insight, LiveEvent, LiveResponse, Race, Snapshot, Status } from '@/lib/live';
import { lapTime, tyreColors } from './cards';

const speeds = [1, 5, 20, 60];

const flagStyle: Record<Flag, { label: string; className: string }> = {
  green: { label: 'Green flag', className: 'bg-emerald-600/20 text-emerald-300 border-emerald-500/40' },
  yellow: { label: 'Yellow flag', className: 'bg-yellow-400/15 text-yellow-300 border-yellow-400/40' },
  vsc: { label: 'Virtual safety car', className: 'bg-amber-500/20 text-amber-300 border-amber-400/50' },
  sc: { label: 'Safety car', className: 'bg-amber-500/25 text-amber-200 border-amber-400/60' },
  red: { label: 'Red flag', className: 'bg-red-600/25 text-red-200 border-red-500/60' },
  chequered: { label: 'Chequered flag', className: 'bg-white/10 text-white border-white/30' },
};

// LiveView follows a session as it happens, or a replay of one: the timing
// tower and what's happening, polled every second.
export function LiveView() {
  const [data, setData] = useState<LiveResponse | null>(null);
  const [events, setEvents] = useState<LiveEvent[]>([]);
  const [insights, setInsights] = useState<Insight[]>([]);
  const [races, setRaces] = useState<Race[]>([]);
  const [race, setRace] = useState('');
  const [speed, setSpeed] = useState(20);
  const [fromLap, setFromLap] = useState(1);
  const [error, setError] = useState('');
  const last = useRef({ run: -1, id: 0, insight: 0 });

  useEffect(() => {
    fetch('/api/live/races')
      .then(r => r.json())
      .then((rs: Race[]) => {
        setRaces(rs);
        if (rs.length) setRace(r => r || rs[0].id);
      })
      .catch(() => setError('Could not load the replays.'));
  }, []);

  const poll = useCallback(async (signal: AbortSignal) => {
    const since = last.current.id;
    const resp = await fetch(`/api/live?since=${since}&isince=${last.current.insight}`, { cache: 'no-store', signal });
    if (!resp.ok) throw new Error(await resp.text());
    const d: LiveResponse = await resp.json();
    if (signal.aborted || d.status.run < last.current.run) return; // stale
    if (d.status.run !== last.current.run) {
      // A new replay: its events are numbered from 1 again, so start over
      // from the first on the next poll.
      last.current = { run: d.status.run, id: 0, insight: 0 };
      setEvents([]);
      setInsights([]);
      setData(d);
      if (since !== 0) return;
    }
    const fresh = (d.events ?? []).filter(e => e.id > last.current.id);
    if (fresh.length) {
      last.current.id = fresh[fresh.length - 1].id;
      const newestFirst = fresh.slice().reverse();
      setEvents(prev => [...newestFirst, ...prev].slice(0, 400));
    }
    const newInsights = (d.insights ?? []).filter(i => i.id > last.current.insight);
    if (newInsights.length) {
      last.current.insight = newInsights[newInsights.length - 1].id;
      const newestFirst = newInsights.slice().reverse();
      setInsights(prev => [...newestFirst, ...prev].slice(0, 200));
    }
    setData(d);
  }, []);

  useEffect(() => {
    const ctrl = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      try {
        await poll(ctrl.signal);
        setError('');
      } catch {
        if (ctrl.signal.aborted) return;
        setError('Lost touch with the timing feed. Retrying…');
      }
      if (!ctrl.signal.aborted) timer = setTimeout(tick, 1000);
    };
    tick();
    return () => {
      ctrl.abort();
      clearTimeout(timer);
    };
  }, [poll]);

  const control = async (body: object) => {
    const resp = await fetch('/api/live/replay', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!resp.ok) setError((await resp.json().catch(() => ({}))).error ?? 'Could not start the replay.');
    else setError('');
  };

  const snap = data?.snapshot;
  const status = data?.status;
  const laps = races.find(r => r.id === race)?.laps ?? 0;

  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-10 border-b border-line bg-bg/90 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3">
          <div className="h-5 w-1.5 rounded-sm bg-accent" />
          <h1 className="text-lg font-semibold tracking-tight">Pit Wall</h1>
          <nav className="ml-4 flex gap-1 text-sm">
            <Link href="/" className="rounded-md px-2.5 py-1 text-muted hover:bg-panel hover:text-text">Chat</Link>
            <span className="rounded-md bg-panel px-2.5 py-1">Live</span>
          </nav>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-5">
        <Controls
          races={races}
          race={race}
          setRace={setRace}
          speed={speed}
          setSpeed={s => {
            setSpeed(s);
            if (status?.running && status.race) control({ race: status.race, speed: s, from_lap: Math.max(1, snap?.lap ?? 1) });
          }}
          fromLap={fromLap}
          setFromLap={setFromLap}
          laps={laps}
          status={status}
          onStart={() => control({ race, speed, from_lap: fromLap })}
          onStop={() => control({ stop: true })}
        />
        {error && <p className="mt-3 rounded-lg border border-accent/40 bg-accent/10 px-3 py-2 text-sm">{error}</p>}

        {snap?.cars?.length ? (
          <>
            <SessionBar snap={snap} status={status} />
            <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
              <Tower cars={snap.cars} best={snap.best_lap} />
              <Feed events={events} insights={insights} start={status?.start} />
            </div>
          </>
        ) : (
          <p className="mt-10 text-center text-muted">Nothing is playing. Pick a race and press Play to follow it as if it were live.</p>
        )}
      </main>
    </div>
  );
}

function Controls(p: {
  races: Race[];
  race: string;
  setRace: (r: string) => void;
  speed: number;
  setSpeed: (s: number) => void;
  fromLap: number;
  setFromLap: (n: number) => void;
  laps: number;
  status?: Status;
  onStart: () => void;
  onStop: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-line bg-panel px-4 py-3 text-sm">
      <span className="rounded bg-panel-2 px-2 py-0.5 text-xs font-semibold uppercase tracking-wide text-muted">Replay</span>
      <select value={p.race} onChange={e => p.setRace(e.target.value)} className="rounded-md border border-line bg-bg px-2 py-1">
        {p.races.map(r => (
          <option key={r.id} value={r.id}>
            {r.title}
          </option>
        ))}
      </select>
      <label className="flex items-center gap-2 text-muted">
        from lap
        <input
          type="number"
          min={1}
          max={p.laps || undefined}
          value={p.fromLap}
          onChange={e => p.setFromLap(Math.max(1, Number(e.target.value) || 1))}
          className="w-16 rounded-md border border-line bg-bg px-2 py-1 text-text"
        />
      </label>
      <div className="flex overflow-hidden rounded-md border border-line">
        {speeds.map(s => (
          <button
            key={s}
            onClick={() => p.setSpeed(s)}
            className={`px-2.5 py-1 tabular-nums ${p.speed === s ? 'bg-accent text-white' : 'bg-bg text-muted hover:text-text'}`}
          >
            {s}×
          </button>
        ))}
      </div>
      <button onClick={p.onStart} disabled={!p.race} className="rounded-md bg-accent px-3 py-1 font-medium text-white disabled:opacity-40">
        {p.status?.running ? 'Restart' : 'Play'}
      </button>
      {p.status?.running && (
        <button onClick={p.onStop} className="rounded-md border border-line px-3 py-1">
          Stop
        </button>
      )}
    </div>
  );
}

function SessionBar({ snap, status }: { snap: Snapshot; status?: Status }) {
  const flag = flagStyle[snap.flag] ?? flagStyle.green;
  const t = status?.time ?? snap.time;
  return (
    <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2">
      <h2 className="text-xl font-semibold">{status?.title ?? snap.session}</h2>
      <span className="text-lg tabular-nums">{buildUp(status, t) ?? `Lap ${snap.lap}`}</span>
      <span className={`rounded-md border px-2.5 py-0.5 text-sm font-medium ${flag.className}`}>{flag.label}</span>
      {snap.weather && (
        <span className="text-sm text-muted">
          {snap.weather.rain ? 'Rain · ' : ''}Air {snap.weather.air_c.toFixed(0)} °C · Track {snap.weather.track_c.toFixed(0)} °C
        </span>
      )}
      <span className="ml-auto text-sm tabular-nums text-muted">
        {t ? new Date(t).toISOString().slice(11, 19) : ''} UTC{status?.running ? ` · ${status.speed}×` : ' · paused'}
      </span>
    </div>
  );
}

// buildUp describes the time before the scheduled start, e.g. "Build-up ·
// starts in 12 min", or returns null once it has passed.
function buildUp(status: Status | undefined, now?: string): string | null {
  if (!status?.start || !now) return null;
  const mins = (new Date(status.start).getTime() - new Date(now).getTime()) / 60000;
  if (mins <= 0) return null;
  return `Build-up · starts in ${Math.ceil(mins)} min`;
}

function Tower({ cars, best }: { cars: Car[]; best?: Snapshot['best_lap'] }) {
  return (
    <section className="overflow-hidden rounded-xl border border-line bg-panel">
      <table className="w-full text-sm">
        <thead className="text-left text-xs uppercase tracking-wide text-muted">
          <tr className="border-b border-line">
            <th className="w-10 px-3 py-2">Pos</th>
            <th className="px-2 py-2">Driver</th>
            <th className="px-2 py-2 text-right">Gap</th>
            <th className="hidden px-2 py-2 text-right sm:table-cell">Int</th>
            <th className="px-2 py-2 text-right">Last lap</th>
            <th className="px-2 py-2">Tyre</th>
            <th className="hidden px-3 py-2 text-right sm:table-cell">Pits</th>
          </tr>
        </thead>
        <tbody className="tabular-nums">
          {cars.map(c => {
            const fastest = best && best.driver === c.code && c.last_lap?.lap === best.lap;
            const personal = c.last_lap && c.best_lap && c.last_lap.lap === c.best_lap.lap;
            return (
              <tr key={c.number} className={`border-b border-line/50 ${c.out ? 'opacity-45' : ''}`}>
                <td className="px-3 py-1.5 font-semibold">{c.out ? c.out : c.position ?? ''}</td>
                <td className="px-2 py-1.5">
                  <span className="mr-2 inline-block h-3.5 w-1 translate-y-0.5 rounded-sm" style={{ background: c.color ?? '#888' }} />
                  <span className="font-semibold">{c.code}</span>
                  <span className="ml-2 hidden text-muted md:inline">{c.team}</span>
                  {c.penalties?.length ? <span className="ml-2 rounded bg-accent/20 px-1 text-[10px] font-semibold text-accent">PEN</span> : null}
                </td>
                <td className="px-2 py-1.5 text-right">{c.position === 1 ? 'Leader' : c.gap_text || (c.gap_to_leader != null ? `+${c.gap_to_leader.toFixed(3)}` : '')}</td>
                <td className="hidden px-2 py-1.5 text-right text-muted sm:table-cell">{c.position === 1 || c.interval == null ? '' : `+${c.interval.toFixed(3)}`}</td>
                <td className={`px-2 py-1.5 text-right ${fastest ? 'font-semibold text-fuchsia-400' : personal ? 'text-emerald-400' : ''}`}>
                  {c.last_lap ? lapTime(c.last_lap.seconds) : ''}
                </td>
                <td className="px-2 py-1.5">
                  {c.compound && (
                    <span className="inline-flex items-center gap-1.5">
                      <span
                        className="inline-flex h-4 w-4 items-center justify-center rounded-full border-2 text-[9px] font-bold"
                        style={{ borderColor: tyreColors[c.compound] ?? '#888', color: tyreColors[c.compound] ?? '#888' }}
                      >
                        {c.compound[0]}
                      </span>
                      <span className="text-muted">{c.tyre_age}</span>
                    </span>
                  )}
                </td>
                <td className="hidden px-3 py-1.5 text-right sm:table-cell">{c.stops?.length || ''}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}

const kindStyle: Record<string, string> = {
  flag: 'border-l-amber-400',
  penalty: 'border-l-accent',
  lead: 'border-l-white',
  pit: 'border-l-sky-400',
  fastest_lap: 'border-l-fuchsia-400',
  overtake: 'border-l-emerald-400',
  closing: 'border-l-orange-300',
  pace: 'border-l-orange-300',
  weather: 'border-l-blue-300',
  radio: 'border-l-muted',
};

const topicLabel: Record<string, string> = {
  welcome: 'Welcome',
  grid: 'The grid',
  last_year: 'Last year',
  strategy: 'Strategy',
  championship: 'Championship',
  watch: 'One to watch',
};

type FeedItem = { key: string; time: string; lap?: number } & ({ insight: Insight } | { event: LiveEvent });

function Feed({ events, insights, start }: { events: LiveEvent[]; insights: Insight[]; start?: string }) {
  const [all, setAll] = useState(false);
  const items: FeedItem[] = [
    ...insights.map(i => ({ key: `i${i.id}`, time: i.time, lap: i.lap, insight: i })),
    ...events.filter(e => all || e.priority >= 2).map(e => ({ key: `e${e.id}`, time: e.time, lap: e.lap, event: e })),
  ].sort((a, b) => (a.time < b.time ? 1 : a.time > b.time ? -1 : 'insight' in a ? -1 : 1));
  return (
    <section className="flex max-h-[calc(100dvh-14rem)] flex-col overflow-hidden rounded-xl border border-line bg-panel">
      <header className="flex items-center border-b border-line px-4 py-2.5">
        <h3 className="font-semibold">What’s happening</h3>
        <label className="ml-auto flex items-center gap-2 text-xs text-muted">
          <input type="checkbox" checked={all} onChange={e => setAll(e.target.checked)} />
          everything
        </label>
      </header>
      <ul className="flex-1 space-y-1.5 overflow-auto p-3">
        {items.length === 0 && <li className="px-1 text-sm text-muted">Nothing yet.</li>}
        {items.map(item =>
          'insight' in item ? (
            <li
              key={item.key}
              className={`rounded-lg border px-3 py-2 text-sm ${item.insight.kind === 'preview' ? 'border-sky-400/35 bg-sky-400/10' : 'border-accent/35 bg-accent/10'}`}
            >
              <div className="mb-0.5 flex items-center gap-2 text-xs">
                {item.insight.kind === 'preview' ? (
                  <span className="font-semibold uppercase tracking-wide text-sky-300">
                    Pre-race{item.insight.topic ? ` · ${topicLabel[item.insight.topic] ?? item.insight.topic}` : ''}
                  </span>
                ) : (
                  <span className="font-semibold uppercase tracking-wide text-accent">Pit wall</span>
                )}
                <span className="tabular-nums text-muted">
                  {item.insight.kind === 'preview' && start
                    ? `${Math.max(0, Math.round((new Date(start).getTime() - new Date(item.time).getTime()) / 60000))} min to start`
                    : item.lap
                      ? `Lap ${item.lap}`
                      : ''}
                </span>
              </div>
              <p className="leading-relaxed">{item.insight.text}</p>
            </li>
          ) : (
            <li
              key={item.key}
              className={`rounded-md border-l-2 bg-bg/60 px-3 py-1.5 text-sm ${kindStyle[item.event.kind] ?? 'border-l-line'} ${item.event.priority >= 3 ? 'font-medium' : ''}`}
            >
              <span className="mr-2 text-xs tabular-nums text-muted">{item.lap ? `L${item.lap}` : new Date(item.time).toISOString().slice(11, 16)}</span>
              {item.event.text}
            </li>
          ),
        )}
      </ul>
    </section>
  );
}
