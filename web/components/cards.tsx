/* eslint-disable @next/next/no-img-element -- images come from the Go
   service through /img, already sized; next/image adds nothing here. */
import type { ReactNode } from 'react';
import type { ImageLink, ToolOutput } from '@/lib/f1api';
import type { ChatMessage } from '@/lib/types';

type Part = ChatMessage['parts'][number];
type ToolPart = Extract<Part, { type: `tool-${string}` }>;

export function isToolPart(p: Part): p is ToolPart {
  return p.type.startsWith('tool-');
}

// Result shapes from the Go service, as far as the cards use them.
type DriverRef = { code?: string; number?: number; name: string };
type Event = { year: number; round: number; name: string; circuit: { id: string; name: string; locality: string; country: string }; sprint: boolean; sessions: { session: string; start: string }[] };

// ToolCard draws one tool call: what it's doing while it runs, then its
// answer as a card.
export function ToolCard({ part }: { part: ToolPart }) {
  const name = part.type.slice('tool-'.length);
  switch (part.state) {
    case 'input-streaming':
    case 'input-available':
      return <Progress text={progressText(name, (part.input ?? {}) as Record<string, unknown>)} />;
    case 'output-error':
      return (
        <div className="rounded-lg border border-line bg-panel px-3 py-2 text-sm text-muted">
          <span className="text-accent">●</span> {part.errorText}
        </div>
      );
    case 'output-available':
      return <Answer name={name} output={part.output as ToolOutput} input={(part.input ?? {}) as Record<string, unknown>} />;
    default:
      return null;
  }
}

function Progress({ text }: { text: string }) {
  return (
    <div className="flex items-center gap-2 text-sm text-muted">
      <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-accent" />
      {text}
    </div>
  );
}

function progressText(name: string, input: Record<string, unknown>): string {
  const where = [input.year, input.round].filter(Boolean).join(' ');
  const session = typeof input.session === 'string' ? ` ${input.session.toUpperCase()}` : '';
  const laps = Array.isArray(input.laps)
    ? (input.laps as { driver: string; lap?: string }[]).map(l => `${l.driver}${l.lap ? ` (${l.lap.replace('_', ' ')})` : ''}`).join(' vs ')
    : '';
  switch (name) {
    case 'schedule': return `Getting the ${input.year ?? 'current'} calendar…`;
    case 'sessionResults': return `Getting results${where ? ` for ${where}` : ''}${session}…`;
    case 'standings': return 'Getting the standings…';
    case 'drivers': return 'Getting the drivers…';
    case 'laps': return `Getting lap times${where ? ` for ${where}` : ''}${session}…`;
    case 'raceOrder': return 'Working out the running order…';
    case 'pitStops': return 'Getting pit stops…';
    case 'tyreStints': return 'Getting tyre stints…';
    case 'raceControl': return 'Reading race control messages…';
    case 'weather': return 'Getting the weather…';
    case 'carTelemetry': return `Getting ${input.driver}'s telemetry for lap ${input.lap}…`;
    case 'track': return `Drawing ${input.circuit ?? input.round ?? 'the track'}${input.corner ? `, turn ${input.corner}` : ''}…`;
    case 'incident': return `Working out what caused the ${input.event === 'sc' ? 'safety car' : input.event === 'vsc' ? 'virtual safety car' : input.event === 'yellow' ? 'yellow flag' : 'red flag'}…`;
    case 'compareLaps': return `Comparing ${laps || 'the fastest laps'}${session} — fetching laps from OpenF1…`;
    case 'lapReplay': return `${input.animate ? 'Animating' : 'Replaying'} ${laps || 'the fastest lap'}${session}…`;
    default: return 'Working…';
  }
}

function Answer({ name, output, input }: { name: string; output: ToolOutput; input: Record<string, unknown> }) {
  const r = output.result as never;
  const images = output.images ?? [];
  switch (name) {
    case 'schedule': return <ScheduleCard r={r} />;
    case 'sessionResults': return <ResultsCard r={r} />;
    case 'standings': return <StandingsCard r={r} />;
    case 'laps': return <LapsCard r={r} />;
    case 'raceOrder': return <RaceOrderCard r={r} />;
    case 'pitStops': return <PitStopsCard r={r} />;
    case 'tyreStints': return <StintsCard r={r} />;
    case 'raceControl': return <RaceControlCard r={r} />;
    case 'weather': return <WeatherCard r={r} />;
    case 'carTelemetry': return <TelemetryCard r={r} />;
    case 'track': return <TrackCard r={r} images={images} corner={input.corner as number | undefined} />;
    case 'incident': return <IncidentCard r={r} images={images} />;
    case 'compareLaps': return <CompareCard r={r} images={images} />;
    case 'lapReplay': return <ReplayCard r={r} images={images} />;
    default: return <RawCard value={output.result} />;
  }
}

// Shared pieces.

function Card({ title, subtitle, children }: { title: ReactNode; subtitle?: ReactNode; children: ReactNode }) {
  return (
    <section className="overflow-hidden rounded-xl border border-line bg-panel">
      <header className="flex items-baseline gap-2 border-b border-line px-4 py-2.5">
        <h3 className="font-semibold">{title}</h3>
        {subtitle && <span className="truncate text-sm text-muted">{subtitle}</span>}
      </header>
      {children}
    </section>
  );
}

function Table({ head, rows, align }: { head: string[]; rows: ReactNode[][]; align?: ('l' | 'r')[] }) {
  return (
    <div className="max-h-[28rem] overflow-auto">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-panel text-left text-xs uppercase tracking-wide text-muted">
          <tr>
            {head.map((h, i) => (
              <th key={i} className={`px-3 py-2 font-medium ${align?.[i] === 'r' ? 'text-right' : ''}`}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody className="tabular-nums">
          {rows.map((row, i) => (
            <tr key={i} className="border-t border-line/60">
              {row.map((c, j) => (
                <td key={j} className={`px-3 py-1.5 ${align?.[j] === 'r' ? 'text-right' : ''}`}>{c}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Img({ img, alt }: { img?: ImageLink; alt: string }) {
  if (!img) return null;
  return (
    <a href={img.url} target="_blank" rel="noreferrer">
      <img src={img.url} alt={alt} className="block w-full bg-bg" />
    </a>
  );
}

function Who({ d }: { d?: DriverRef }) {
  if (!d) return null;
  return (
    <span title={d.name}>
      <span className="font-medium">{d.code ?? d.name}</span>
      {d.code && <span className="ml-1.5 hidden text-muted sm:inline">{d.name}</span>}
    </span>
  );
}

// lapTime formats seconds as a lap or race time: 88.111 as 1:28.111, and
// 5882.143 as 1:38:02.143.
export function lapTime(s?: number | null) {
  if (s == null) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s - h * 3600) / 60);
  const rest = (s - h * 3600 - m * 60).toFixed(3).padStart(6, '0');
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${rest}`;
  return m > 0 ? `${m}:${rest}` : rest;
}

const gap = (s?: number | null) => (s == null ? '' : `+${s.toFixed(3)}`);

const tyreColors: Record<string, string> = {
  SOFT: '#DA291C', MEDIUM: '#FFD12E', HARD: '#F0F0EC', INTERMEDIATE: '#43B02A', WET: '#0067AD',
};

function Tyre({ compound, age }: { compound?: string; age?: number }) {
  if (!compound) return null;
  return (
    <span className="inline-flex items-center gap-1 text-muted">
      <span
        className="inline-flex h-4 w-4 items-center justify-center rounded-full border-2 text-[9px] font-bold"
        style={{ borderColor: tyreColors[compound] ?? '#888', color: tyreColors[compound] ?? '#888' }}
      >
        {compound[0]}
      </span>
      {age != null && <span className="text-xs">{age}</span>}
    </span>
  );
}

function eventTitle(e?: Event) {
  return e ? `${e.year} ${e.name}` : '';
}

// Cards for each tool.

function ScheduleCard({ r }: { r: { year: number; events: Event[] } }) {
  return (
    <Card title={`${r.year} calendar`} subtitle={`${r.events.length} rounds`}>
      <Table
        head={['Rd', 'Event', 'Circuit', 'Race']}
        rows={r.events.map(e => {
          const race = e.sessions.find(s => s.session === 'race')?.start;
          return [
            e.round,
            <span key="n">{e.name}{e.sprint && <span className="ml-1.5 rounded bg-panel-2 px-1 text-[10px] uppercase text-muted">sprint</span>}</span>,
            <span key="c" className="text-muted">{e.circuit.locality}, {e.circuit.country}</span>,
            race ? new Date(race).toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) : '—',
          ];
        })}
      />
    </Card>
  );
}

type SessionResult = {
  position?: number; classified?: string; driver: DriverRef; team?: string; grid?: number; laps?: number; status?: string; points?: number;
  time_seconds?: number; gap_seconds?: number; q1_seconds?: number; q2_seconds?: number; q3_seconds?: number; best_lap_seconds?: number;
};

function ResultsCard({ r }: { r: { event: Event; session: string; results: SessionResult[] } }) {
  const quali = r.results.some(x => x.q1_seconds != null);
  const practice = r.results.some(x => x.best_lap_seconds != null) && !quali;
  const title = `${eventTitle(r.event)} · ${r.session.replace('_', ' ')}`;
  if (quali) {
    return (
      <Card title={title}>
        <Table
          head={['Pos', 'Driver', 'Team', 'Q1', 'Q2', 'Q3']}
          align={['l', 'l', 'l', 'r', 'r', 'r']}
          rows={r.results.map(x => [x.position ?? x.classified, <Who key="d" d={x.driver} />, <span key="t" className="text-muted">{x.team}</span>, lapTime(x.q1_seconds), x.q2_seconds ? lapTime(x.q2_seconds) : '', x.q3_seconds ? lapTime(x.q3_seconds) : ''])}
        />
      </Card>
    );
  }
  if (practice) {
    return (
      <Card title={title}>
        <Table
          head={['Pos', 'Driver', 'Team', 'Best', 'Gap']}
          align={['l', 'l', 'l', 'r', 'r']}
          rows={r.results.map(x => [x.position, <Who key="d" d={x.driver} />, <span key="t" className="text-muted">{x.team}</span>, lapTime(x.best_lap_seconds), gap(x.gap_seconds)])}
        />
      </Card>
    );
  }
  return (
    <Card title={title}>
      <Table
        head={['Pos', 'Driver', 'Team', 'Grid', 'Time / gap', 'Pts']}
        align={['l', 'l', 'l', 'r', 'r', 'r']}
        rows={r.results.map(x => [
          x.position ?? x.classified,
          <Who key="d" d={x.driver} />,
          <span key="t" className="text-muted">{x.team}</span>,
          x.grid || 'PL',
          x.time_seconds != null ? lapTime(x.time_seconds) : x.gap_seconds != null ? gap(x.gap_seconds) : <span key="s" className="text-muted">{x.status}</span>,
          x.points || '',
        ])}
      />
    </Card>
  );
}

function StandingsCard({ r }: { r: { year: number; after_round: number; championship: string; standings: { position: number; driver?: DriverRef; team: string; points: number; wins: number }[] } }) {
  const drivers = r.championship !== 'teams';
  return (
    <Card title={`${r.year} ${drivers ? "drivers'" : 'teams'} championship`} subtitle={`after round ${r.after_round}`}>
      <Table
        head={drivers ? ['Pos', 'Driver', 'Team', 'Wins', 'Pts'] : ['Pos', 'Team', 'Wins', 'Pts']}
        align={drivers ? ['l', 'l', 'l', 'r', 'r'] : ['l', 'l', 'r', 'r']}
        rows={r.standings.map(s =>
          drivers
            ? [s.position, <Who key="d" d={s.driver} />, <span key="t" className="text-muted">{s.team}</span>, s.wins || '', <b key="p">{s.points}</b>]
            : [s.position, s.team, s.wins || '', <b key="p">{s.points}</b>],
        )}
      />
    </Card>
  );
}

type LapTimeRow = { driver: DriverRef; lap: number; seconds?: number; sectors?: (number | null)[]; pit_out_lap?: boolean; position?: number; gap_seconds?: number; laps?: number };

function LapsCard({ r }: { r: { event: Event; session: string; best_laps?: LapTimeRow[]; laps?: LapTimeRow[]; page?: { total: number } } }) {
  const title = `${eventTitle(r.event)} · ${r.session.toUpperCase()}`;
  const sectors = (x: LapTimeRow) => (x.sectors ?? [null, null, null]).map(s => (s == null ? '—' : s.toFixed(3)));
  if (r.best_laps) {
    return (
      <Card title={title} subtitle="best laps">
        <Table
          head={['Pos', 'Driver', 'Lap', 'Time', 'Gap', 'S1', 'S2', 'S3']}
          align={['l', 'l', 'r', 'r', 'r', 'r', 'r', 'r']}
          rows={r.best_laps.map(x => [x.position, <Who key="d" d={x.driver} />, x.lap, <b key="t">{lapTime(x.seconds)}</b>, gap(x.gap_seconds) || '', ...sectors(x)])}
        />
      </Card>
    );
  }
  return (
    <Card title={title} subtitle={`${r.page?.total ?? r.laps?.length ?? 0} laps`}>
      <Table
        head={['Driver', 'Lap', 'Time', 'S1', 'S2', 'S3']}
        align={['l', 'r', 'r', 'r', 'r', 'r']}
        rows={(r.laps ?? []).map(x => [<Who key="d" d={x.driver} />, <span key="l">{x.lap}{x.pit_out_lap && <span className="ml-1 text-xs text-muted">out</span>}</span>, lapTime(x.seconds), ...sectors(x)])}
      />
    </Card>
  );
}

function RaceOrderCard({ r }: { r: { event: Event; session: string; lap: number; of: number; order: { position: number; driver: DriverRef; team?: string; gap_seconds?: number; interval_seconds?: number; laps_down?: number }[] } }) {
  return (
    <Card title={`${eventTitle(r.event)} · lap ${r.lap} of ${r.of}`}>
      <Table
        head={['Pos', 'Driver', 'Team', 'Gap', 'Interval']}
        align={['l', 'l', 'l', 'r', 'r']}
        rows={r.order.map(o => [
          o.position,
          <Who key="d" d={o.driver} />,
          <span key="t" className="text-muted">{o.team}</span>,
          o.laps_down ? `+${o.laps_down} lap${o.laps_down > 1 ? 's' : ''}` : o.position === 1 ? 'Leader' : gap(o.gap_seconds),
          o.position === 1 || o.laps_down ? '' : gap(o.interval_seconds),
        ])}
      />
    </Card>
  );
}

function PitStopsCard({ r }: { r: { event: Event; pit_stops: { driver: DriverRef; stop: number; lap: number; pit_lane_seconds?: number; stationary_seconds?: number; compound_before?: string; compound_after?: string }[] } }) {
  return (
    <Card title={`${eventTitle(r.event)} · pit stops`} subtitle={`${r.pit_stops.length} stops`}>
      <Table
        head={['Driver', 'Stop', 'Lap', 'Pit lane', 'Stationary', 'Tyres']}
        align={['l', 'r', 'r', 'r', 'r', 'l']}
        rows={r.pit_stops.map(p => [
          <Who key="d" d={p.driver} />, p.stop, p.lap,
          p.pit_lane_seconds?.toFixed(3) ?? '—', p.stationary_seconds?.toFixed(1) ?? '—',
          <span key="c" className="inline-flex items-center gap-1"><Tyre compound={p.compound_before} /> → <Tyre compound={p.compound_after} /></span>,
        ])}
      />
    </Card>
  );
}

// StintsCard draws each driver's stints as coloured bars across the laps.
function StintsCard({ r }: { r: { event: Event; session: string; stints: { driver: DriverRef; stint: number; compound: string; lap_start: number; lap_end: number; tyre_age_at_start: number }[] } }) {
  const total = Math.max(1, ...r.stints.map(s => s.lap_end));
  const byDriver = new Map<string, typeof r.stints>();
  for (const s of r.stints) {
    const k = s.driver.code ?? s.driver.name;
    byDriver.set(k, [...(byDriver.get(k) ?? []), s]);
  }
  return (
    <Card title={`${eventTitle(r.event)} · tyre stints`} subtitle={`${total} laps`}>
      <div className="space-y-1.5 px-4 py-3">
        {[...byDriver].map(([who, stints]) => (
          <div key={who} className="flex items-center gap-2 text-xs">
            <span className="w-10 font-medium">{who}</span>
            <div className="relative h-4 flex-1 rounded bg-bg">
              {stints.map(s => (
                <div
                  key={s.stint}
                  title={`${s.compound}, laps ${s.lap_start}–${s.lap_end}${s.tyre_age_at_start ? `, ${s.tyre_age_at_start} laps old` : ''}`}
                  className="absolute top-0 h-4 rounded-sm border border-bg"
                  style={{ left: `${((s.lap_start - 1) / total) * 100}%`, width: `${((s.lap_end - s.lap_start + 1) / total) * 100}%`, background: tyreColors[s.compound] ?? '#888' }}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    </Card>
  );
}

function RaceControlCard({ r }: { r: { event: Event; session: string; messages: { time: string; lap?: number; category: string; flag?: string; message: string }[] } }) {
  const flagColor = (f?: string) => (f === 'RED' ? '#E10600' : f?.includes('YELLOW') ? '#FFD100' : f === 'GREEN' ? '#2ECC71' : f === 'CHEQUERED' ? '#fff' : '#9A9AA8');
  return (
    <Card title={`${eventTitle(r.event)} · race control`} subtitle={`${r.messages.length} messages`}>
      <ul className="max-h-[28rem] divide-y divide-line/60 overflow-auto text-sm">
        {r.messages.map((m, i) => (
          <li key={i} className="flex gap-3 px-4 py-1.5">
            <span className="w-12 shrink-0 tabular-nums text-muted">{m.lap ? `L${m.lap}` : new Date(m.time).toISOString().slice(11, 16)}</span>
            <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full" style={{ background: flagColor(m.flag) }} />
            <span>{m.message}</span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

function WeatherCard({ r }: { r: { event: Event; session: string; summary: { air_temp_c: Range; track_temp_c: Range; humidity_pct: Range; wind_speed_m_s: Range; rain: boolean } } }) {
  const s = r.summary;
  const stat = (label: string, v: Range, unit: string, digits = 0) => (
    <div className="rounded-lg bg-bg px-3 py-2">
      <div className="text-xs text-muted">{label}</div>
      <div className="text-lg font-semibold tabular-nums">{v.avg.toFixed(digits)}{unit}</div>
      <div className="text-xs text-muted tabular-nums">{v.min.toFixed(digits)}–{v.max.toFixed(digits)}</div>
    </div>
  );
  return (
    <Card title={`${eventTitle(r.event)} · weather`} subtitle={r.session}>
      <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-5">
        {stat('Air', s.air_temp_c, ' °C')}
        {stat('Track', s.track_temp_c, ' °C')}
        {stat('Humidity', s.humidity_pct, '%')}
        {stat('Wind', s.wind_speed_m_s, ' m/s', 1)}
        <div className="rounded-lg bg-bg px-3 py-2">
          <div className="text-xs text-muted">Rain</div>
          <div className="text-lg font-semibold">{s.rain ? 'Yes' : 'Dry'}</div>
        </div>
      </div>
    </Card>
  );
}
type Range = { min: number; max: number; avg: number };

function TelemetryCard({ r }: { r: { event: Event; session: string; driver: DriverRef; lap: number; lap_seconds?: number; top_speed_kph: number; top_speed_where?: string; min_speed_kph: number; full_throttle_pct: number; braking_pct: number; gear_changes: number; braking: { from_kph: number; to_kph: number; seconds: number; corner?: string }[] } }) {
  return (
    <Card title={`${r.driver.code ?? r.driver.name} · lap ${r.lap} · ${lapTime(r.lap_seconds)}`} subtitle={`${eventTitle(r.event)} ${r.session}`}>
      <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-4">
        {[
          ['Top speed', `${r.top_speed_kph} km/h`, r.top_speed_where],
          ['Slowest', `${r.min_speed_kph} km/h`, ''],
          ['Full throttle', `${r.full_throttle_pct.toFixed(0)}%`, `braking ${r.braking_pct.toFixed(0)}%`],
          ['Gear changes', r.gear_changes, ''],
        ].map(([l, v, sub]) => (
          <div key={l as string} className="rounded-lg bg-bg px-3 py-2">
            <div className="text-xs text-muted">{l}</div>
            <div className="text-lg font-semibold tabular-nums">{v}</div>
            {sub && <div className="truncate text-xs text-muted">{sub}</div>}
          </div>
        ))}
      </div>
      <Table
        head={['Braking for', 'From', 'To', 'Time']}
        align={['l', 'r', 'r', 'r']}
        rows={r.braking.map(b => [b.corner ?? '—', `${b.from_kph}`, `${b.to_kph}`, `${b.seconds.toFixed(1)} s`])}
      />
    </Card>
  );
}

function TrackCard({ r, images, corner }: { r: { name: string; locality: string; country: string; layout: string; length_km: number; turns: number; elevation_change_m?: number; corners: { number: number; name?: string }[] }; images: ImageLink[]; corner?: number }) {
  const c = corner ? r.corners.find(x => x.number === corner) : undefined;
  return (
    <Card title={c ? `Turn ${c.number}${c.name ? ` · ${c.name}` : ''}` : r.name} subtitle={`${r.locality}, ${r.country} · ${r.length_km.toFixed(3)} km · ${r.turns} turns${r.elevation_change_m ? ` · ${r.elevation_change_m.toFixed(0)} m climb` : ''}`}>
      <Img img={images[0]} alt={`Map of ${r.name}`} />
    </Card>
  );
}

function IncidentCard({ r, images }: { r: { event: Event; session: string; banner: string; caption?: string; flags?: string[] }; images: ImageLink[] }) {
  return (
    <Card title={r.banner} subtitle={eventTitle(r.event)}>
      {r.caption && <p className="px-4 py-2 text-sm">{r.caption}</p>}
      <div className="grid gap-px bg-line sm:grid-cols-2">
        {images.map(img => (
          <Img key={img.url} img={img} alt={img.name === 'corner' ? 'Close-up of the nearest turn' : 'Track map'} />
        ))}
      </div>
    </Card>
  );
}

type LapSummary = { driver: string; number: number; lap: number; seconds: number; gap_seconds?: number; compound?: string; tyre_age_laps?: number; top_speed_kph: number; stretches_won?: number; biggest_gain?: { stretch: string; seconds: number } };

function LapChips({ laps }: { laps: LapSummary[] }) {
  return (
    <div className="flex flex-wrap gap-2 px-4 py-3">
      {laps.map(l => (
        <div key={l.driver} className="rounded-lg bg-bg px-3 py-2 text-sm">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{l.driver}</span>
            <span className="text-muted">lap {l.lap}</span>
            <Tyre compound={l.compound} age={l.tyre_age_laps} />
          </div>
          <div className="mt-0.5 tabular-nums">
            <b>{lapTime(l.seconds)}</b>
            {l.gap_seconds != null && <span className="ml-2 text-accent">{gap(l.gap_seconds)}</span>}
            <span className="ml-2 text-muted">{l.top_speed_kph} km/h</span>
          </div>
          {l.stretches_won != null && (
            <div className="mt-0.5 text-xs text-muted">
              fastest in {l.stretches_won}
              {l.biggest_gain && <> · most at {l.biggest_gain.stretch} ({l.biggest_gain.seconds.toFixed(3)} s)</>}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

function CompareCard({ r, images }: { r: { event: Event; session: string; laps: LapSummary[]; stretches?: { name: string; fastest: string; margin_seconds: number; seconds: number[] }[] }; images: ImageLink[] }) {
  return (
    <Card title={r.laps.map(l => l.driver).join(' vs ')} subtitle={`${eventTitle(r.event)} · ${r.session.toUpperCase()}`}>
      <Img img={images.find(i => i.name === 'faster') ?? images[0]} alt="Where each driver was faster" />
      <LapChips laps={r.laps} />
      {r.stretches && (
        <details className="border-t border-line">
          <summary className="cursor-pointer px-4 py-2 text-sm text-muted">Corner by corner</summary>
          <Table
            head={['Stretch', ...r.laps.map(l => l.driver), 'Faster']}
            align={['l', ...r.laps.map(() => 'r' as const), 'l']}
            rows={r.stretches.map(s => [s.name, ...s.seconds.map(x => x.toFixed(3)), <span key="f">{s.fastest} <span className="text-muted">{s.margin_seconds.toFixed(3)}</span></span>])}
          />
        </details>
      )}
    </Card>
  );
}

function ReplayCard({ r, images }: { r: { event: Event; session: string; laps: LapSummary[]; weather?: { air_c: number; track_c: number; rain: boolean } }; images: ImageLink[] }) {
  return (
    <Card title={r.laps.map(l => `${l.driver} ${lapTime(l.seconds)}`).join(' vs ')} subtitle={`${eventTitle(r.event)} · ${r.session.toUpperCase()}`}>
      <Img img={images[0]} alt="Lap replay" />
      <LapChips laps={r.laps} />
    </Card>
  );
}

function RawCard({ value }: { value: unknown }) {
  return (
    <details className="rounded-xl border border-line bg-panel">
      <summary className="cursor-pointer px-4 py-2 text-sm text-muted">Data</summary>
      <pre className="max-h-96 overflow-auto px-4 pb-3 text-xs">{JSON.stringify(value, null, 2)}</pre>
    </details>
  );
}
