'use client';

import { useChat } from '@ai-sdk/react';
import { DefaultChatTransport } from 'ai';
import { useEffect, useRef, useState } from 'react';
import type { ChatMessage } from '@/lib/types';
import { isToolPart, ToolCard } from './cards';
import { Markdown } from './markdown';

const modelChoices = [
  { value: 'auto', label: 'Auto' },
  { value: 'sonnet', label: 'Sonnet 5' },
  { value: 'opus', label: 'Opus 5.5' },
  { value: 'fable', label: 'Fable 5.1' },
] as const;

const examples = [
  'Who won the last race, and how did the top five finish?',
  'Compare Verstappen and Norris in Q3 at Spa 2026: where was each faster?',
  'What caused the red flag at Monaco 2024?',
  'Replay Antonelli’s pole lap at Silverstone 2026 as a GIF',
  'What did Verstappen say on the radio during the 2026 British Grand Prix?',
  'How did the championship look after round 10 this year?',
];

export function Chat() {
  const [input, setInput] = useState('');
  const [model, setModel] = useState<(typeof modelChoices)[number]['value']>('auto');
  const { messages, sendMessage, status, stop, error } = useChat<ChatMessage>({
    transport: new DefaultChatTransport({ api: '/api/chat' }),
  });
  const busy = status === 'submitted' || status === 'streaming';

  // Follow the conversation as it grows, including when images finish
  // loading, unless the reader has scrolled up to look at something.
  const following = useRef(true);
  useEffect(() => {
    const onScroll = () => {
      following.current = window.innerHeight + window.scrollY >= document.body.scrollHeight - 160;
    };
    window.addEventListener('scroll', onScroll, { passive: true });
    const grow = new ResizeObserver(() => {
      if (following.current) window.scrollTo({ top: document.body.scrollHeight });
    });
    grow.observe(document.body);
    return () => {
      window.removeEventListener('scroll', onScroll);
      grow.disconnect();
    };
  }, []);

  const send = (text: string) => {
    if (!text.trim() || busy) return;
    following.current = true;
    sendMessage({ text }, { body: { model } });
    setInput('');
  };

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-10 border-b border-line bg-bg/90 backdrop-blur">
        <div className="mx-auto flex max-w-3xl items-center gap-3 px-4 py-3">
          <div className="h-5 w-1.5 rounded-sm bg-accent" />
          <h1 className="text-lg font-semibold tracking-tight">Pit Wall</h1>
          <span className="hidden text-sm text-muted sm:inline">Formula 1, from the data</span>
          <label className="ml-auto flex items-center gap-2 text-sm text-muted">
            Model
            <select
              value={model}
              onChange={e => setModel(e.target.value as typeof model)}
              className="rounded-md border border-line bg-panel px-2 py-1 text-text"
            >
              {modelChoices.map(m => (
                <option key={m.value} value={m.value}>
                  {m.label}
                </option>
              ))}
            </select>
          </label>
        </div>
      </header>

      <main className="mx-auto w-full max-w-3xl flex-1 px-4 pb-40 pt-6">
        {messages.length === 0 && (
          <div className="mt-10">
            <h2 className="text-2xl font-semibold">What do you want to know?</h2>
            <p className="mt-2 text-muted">
              Results and standings from 1950; laps, telemetry, incidents, lap comparisons and replays from 2023.
            </p>
            <div className="mt-6 grid gap-2 sm:grid-cols-2">
              {examples.map(e => (
                <button
                  key={e}
                  onClick={() => send(e)}
                  className="rounded-lg border border-line bg-panel px-3 py-2.5 text-left text-sm text-text/90 transition hover:border-accent/60 hover:bg-panel-2"
                >
                  {e}
                </button>
              ))}
            </div>
          </div>
        )}

        <div className="space-y-6">
          {messages.map(m =>
            m.role === 'user' ? (
              <div key={m.id} className="flex justify-end">
                <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-panel-2 px-4 py-2.5 text-[15px]">
                  {m.parts.map((p, i) => (p.type === 'text' ? <span key={i}>{p.text}</span> : null))}
                </div>
              </div>
            ) : (
              <div key={m.id} className="space-y-3">
                {m.parts.map((p, i) => {
                  if (p.type === 'text') return <Markdown key={i} text={p.text} />;
                  if (isToolPart(p)) return <ToolCard key={i} part={p} />;
                  return null;
                })}
                {m.metadata?.model && (
                  <div className="text-xs text-muted">
                    {m.metadata.model}
                    {m.metadata.routed ? ' · chosen automatically' : ''}
                  </div>
                )}
              </div>
            ),
          )}
          {status === 'submitted' && <div className="animate-pulse text-sm text-muted">Thinking…</div>}
          {error && (
            <div className="rounded-lg border border-accent/50 bg-accent/10 px-3 py-2 text-sm">
              {error.message || 'Something went wrong. Try again.'}
            </div>
          )}
        </div>
      </main>

      <form
        onSubmit={e => {
          e.preventDefault();
          send(input);
        }}
        className="fixed inset-x-0 bottom-0 border-t border-line bg-bg/95 backdrop-blur"
      >
        <div className="mx-auto flex max-w-3xl items-end gap-2 px-4 py-3">
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                send(input);
              }
            }}
            rows={1}
            placeholder="Ask about a race, a driver, a lap…"
            className="max-h-40 min-h-11 flex-1 resize-none rounded-xl border border-line bg-panel px-3 py-2.5 text-[15px] outline-none placeholder:text-muted focus:border-accent/60"
          />
          {busy ? (
            <button type="button" onClick={stop} className="h-11 rounded-xl border border-line bg-panel px-4 text-sm">
              Stop
            </button>
          ) : (
            <button
              type="submit"
              disabled={!input.trim()}
              className="h-11 rounded-xl bg-accent px-4 text-sm font-medium text-white disabled:opacity-40"
            >
              Send
            </button>
          )}
        </div>
      </form>
    </div>
  );
}
