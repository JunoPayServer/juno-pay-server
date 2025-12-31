"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type EventSink, createEventSink, listEventSinks, testEventSink } from "@/lib/api";

type Kind = "webhook" | "kafka" | "nats" | "rabbitmq";

export default function EventSinksPage() {
  const [filterMerchantID, setFilterMerchantID] = useState("");
  const [sinks, setSinks] = useState<EventSink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [merchantID, setMerchantID] = useState("");
  const [kind, setKind] = useState<Kind>("webhook");

  const [webhookURL, setWebhookURL] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [webhookTimeoutMS, setWebhookTimeoutMS] = useState("");

  const [kafkaBrokers, setKafkaBrokers] = useState("");
  const [kafkaTopic, setKafkaTopic] = useState("");

  const [natsURL, setNatsURL] = useState("");
  const [natsSubject, setNatsSubject] = useState("");

  const [rmqURL, setRmqURL] = useState("");
  const [rmqQueue, setRmqQueue] = useState("");

  async function refresh() {
    try {
      setError(null);
      const out = await listEventSinks({ merchant_id: filterMerchantID.trim() || undefined });
      setSinks(out);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  function buildConfig(): Record<string, unknown> {
    switch (kind) {
      case "webhook": {
        const cfg: Record<string, unknown> = { url: webhookURL.trim() };
        if (webhookSecret.trim()) cfg.secret = webhookSecret.trim();
        if (webhookTimeoutMS.trim()) cfg.timeout_ms = Number.parseInt(webhookTimeoutMS.trim(), 10);
        return cfg;
      }
      case "kafka":
        return { brokers: kafkaBrokers.trim(), topic: kafkaTopic.trim() };
      case "nats":
        return { url: natsURL.trim(), subject: natsSubject.trim() };
      case "rabbitmq":
        return { url: rmqURL.trim(), queue: rmqQueue.trim() };
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Event Sinks</h1>
        <p className="mt-1 text-sm text-zinc-600">Webhooks and brokers (Kafka/NATS/RabbitMQ).</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Create Event Sink</h2>

        <form
          className="mt-4 space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            setError(null);
            try {
              await createEventSink({
                merchant_id: merchantID.trim(),
                kind,
                config: buildConfig(),
              });
              await refresh();
            } catch (e) {
              setError(e instanceof Error ? e.message : "create failed");
            }
          }}
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-zinc-700">Merchant ID</label>
              <input
                value={merchantID}
                onChange={(e) => setMerchantID(e.target.value)}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                placeholder="m_..."
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-zinc-700">Kind</label>
              <select
                value={kind}
                onChange={(e) => setKind(e.target.value as Kind)}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              >
                <option value="webhook">webhook</option>
                <option value="kafka">kafka</option>
                <option value="nats">nats</option>
                <option value="rabbitmq">rabbitmq</option>
              </select>
            </div>
          </div>

          {kind === "webhook" ? (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <label className="block text-sm font-medium text-zinc-700">URL</label>
                <input
                  value={webhookURL}
                  onChange={(e) => setWebhookURL(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="https://example.com/webhook"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-zinc-700">Secret (optional)</label>
                <input
                  value={webhookSecret}
                  onChange={(e) => setWebhookSecret(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-zinc-700">Timeout MS (optional)</label>
                <input
                  value={webhookTimeoutMS}
                  onChange={(e) => setWebhookTimeoutMS(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="5000"
                />
              </div>
            </div>
          ) : null}

          {kind === "kafka" ? (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="block text-sm font-medium text-zinc-700">Brokers</label>
                <input
                  value={kafkaBrokers}
                  onChange={(e) => setKafkaBrokers(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="localhost:9092"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-zinc-700">Topic</label>
                <input
                  value={kafkaTopic}
                  onChange={(e) => setKafkaTopic(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="juno.pay.events"
                  required
                />
              </div>
            </div>
          ) : null}

          {kind === "nats" ? (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="block text-sm font-medium text-zinc-700">URL</label>
                <input
                  value={natsURL}
                  onChange={(e) => setNatsURL(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="nats://localhost:4222"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-zinc-700">Subject</label>
                <input
                  value={natsSubject}
                  onChange={(e) => setNatsSubject(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="juno.pay.events"
                  required
                />
              </div>
            </div>
          ) : null}

          {kind === "rabbitmq" ? (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="block text-sm font-medium text-zinc-700">URL</label>
                <input
                  value={rmqURL}
                  onChange={(e) => setRmqURL(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="amqp://guest:guest@localhost:5672/"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-zinc-700">Queue</label>
                <input
                  value={rmqQueue}
                  onChange={(e) => setRmqQueue(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                  placeholder="juno.pay.events"
                  required
                />
              </div>
            </div>
          ) : null}

          <button
            type="submit"
            className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800"
          >
            Create Sink
          </button>
        </form>

        {error ? (
          <div className="mt-4">
            <ErrorBanner message={error} />
          </div>
        ) : null}
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-zinc-950">Sinks</h2>
          <div className="flex items-center gap-2">
            <input
              value={filterMerchantID}
              onChange={(e) => setFilterMerchantID(e.target.value)}
              className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 shadow-sm"
              placeholder="filter merchant_id"
            />
            <button
              type="button"
              onClick={() => refresh()}
              className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50"
            >
              Refresh
            </button>
          </div>
        </div>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : sinks.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No sinks.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Sink</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Merchant</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Kind</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {sinks.map((s) => (
                  <tr key={s.sink_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{s.sink_id}</div>
                      <div className="text-xs text-zinc-500">{s.created_at}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <span className="font-mono text-xs">{s.merchant_id}</span>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{s.kind}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{s.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <button
                        type="button"
                        onClick={async () => {
                          try {
                            await testEventSink(s.sink_id);
                            alert("test delivered");
                          } catch (e) {
                            alert(e instanceof Error ? e.message : "test failed");
                          }
                        }}
                        className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50"
                      >
                        Test
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

