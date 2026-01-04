"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import {
  APIError,
  type Merchant,
  type MerchantAPIKey,
  type MerchantSettings,
  type MerchantWallet,
  createAPIKey,
  getMerchant,
  revokeAPIKey,
  setMerchantSettings,
  setMerchantWallet,
} from "@/lib/api";

function cloneSettings(s: MerchantSettings): MerchantSettings {
  return JSON.parse(JSON.stringify(s)) as MerchantSettings;
}

export default function MerchantDetailPage() {
  const [merchantID, setMerchantID] = useState("");

  const [merchant, setMerchant] = useState<Merchant | null>(null);
  const [settings, setSettings] = useState<MerchantSettings | null>(null);
  const [wallet, setWallet] = useState<MerchantWallet | null>(null);
  const [apiKeys, setAPIKeys] = useState<MerchantAPIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [walletID, setWalletID] = useState("");
  const [ufvk, setUfVK] = useState("");
  const [chain, setChain] = useState("mainnet");
  const [uaHRP, setUAHRP] = useState("j");
  const [coinType, setCoinType] = useState(8133);

  const [apiKeyLabel, setAPIKeyLabel] = useState("default");
  const [createdKey, setCreatedKey] = useState<{ api_key: string; key_id: string } | null>(null);
  const [revokeKeyID, setRevokeKeyID] = useState("");

  async function refresh(id: string) {
    const v = id.trim();
    if (!v) return;
    try {
      setError(null);
      const m = await getMerchant(v);
      setMerchant(m);
      setSettings(cloneSettings(m.settings));
      setWallet(m.wallet ?? null);
      setAPIKeys(m.api_keys ?? []);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("merchant_id") ?? "";
    setMerchantID(id);
    void refresh(id);
  }, []);

  if (!merchantID) {
    return <div className="text-sm text-zinc-600">merchant_id is required.</div>;
  }
  if (loading && !merchant) {
    return <div className="text-sm text-zinc-600">Loading...</div>;
  }
  if (error) {
    return <ErrorBanner message={error} />;
  }
  if (!merchant || !settings) {
    return <div className="text-sm text-zinc-600">Merchant not found.</div>;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">{merchant.name}</h1>
        <div className="mt-1 font-mono text-xs text-zinc-500">{merchant.merchant_id}</div>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-zinc-950">Settings</h2>
          <button
            type="button"
            onClick={() => refresh(merchant.merchant_id)}
            className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50"
          >
            Refresh
          </button>
        </div>

        <form
          className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
          onSubmit={async (e) => {
            e.preventDefault();
            setError(null);
            try {
              const updated = await setMerchantSettings(merchant.merchant_id, settings);
              setMerchant(updated);
              setSettings(cloneSettings(updated.settings));
            } catch (e) {
              setError(e instanceof Error ? e.message : "save failed");
            }
          }}
        >
          <div>
            <label className="block text-sm font-medium text-zinc-700">Invoice TTL (seconds)</label>
            <input
              type="number"
              min={0}
              value={settings.invoice_ttl_seconds}
              onChange={(e) => setSettings({ ...settings, invoice_ttl_seconds: Number.parseInt(e.target.value || "0", 10) })}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Required Confirmations</label>
            <input
              type="number"
              min={0}
              value={settings.required_confirmations}
              onChange={(e) => setSettings({ ...settings, required_confirmations: Number.parseInt(e.target.value || "0", 10) })}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Late Payment Policy</label>
            <select
              value={settings.policies.late_payment_policy}
              onChange={(e) => setSettings({ ...settings, policies: { ...settings.policies, late_payment_policy: e.target.value } })}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            >
              <option value="mark_paid_late">mark_paid_late</option>
              <option value="manual_review">manual_review</option>
              <option value="ignore">ignore</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Partial Payment Policy</label>
            <select
              value={settings.policies.partial_payment_policy}
              onChange={(e) => setSettings({ ...settings, policies: { ...settings.policies, partial_payment_policy: e.target.value } })}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            >
              <option value="accept_partial">accept_partial</option>
              <option value="reject_partial">reject_partial</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Overpayment Policy</label>
            <select
              value={settings.policies.overpayment_policy}
              onChange={(e) => setSettings({ ...settings, policies: { ...settings.policies, overpayment_policy: e.target.value } })}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            >
              <option value="mark_overpaid">mark_overpaid</option>
              <option value="manual_review">manual_review</option>
            </select>
          </div>

          <div className="sm:col-span-2">
            <button type="submit" className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800">
              Save Settings
            </button>
          </div>
        </form>
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Wallet (UFVK)</h2>
        <p className="mt-1 text-sm text-zinc-600">Immutable once set.</p>

        {wallet ? (
          <div className="mt-4 rounded-md border border-zinc-200 bg-zinc-50 p-3">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">Wallet ID</div>
                <div className="mt-1 font-mono text-xs">{wallet.wallet_id}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">Created At</div>
                <div className="mt-1 text-xs">{wallet.created_at}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">Chain</div>
                <div className="mt-1 text-xs">{wallet.chain}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">UA HRP</div>
                <div className="mt-1 text-xs">{wallet.ua_hrp}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">Coin Type</div>
                <div className="mt-1 text-xs">{wallet.coin_type}</div>
              </div>
            </div>

            <details className="mt-3">
              <summary className="cursor-pointer text-sm font-medium text-zinc-700">UFVK</summary>
              <div className="mt-2 font-mono text-xs break-all text-zinc-800">{wallet.ufvk}</div>
            </details>
          </div>
        ) : (
          <form
            className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
            onSubmit={async (e) => {
              e.preventDefault();
              setError(null);
              try {
                const out = await setMerchantWallet(merchant.merchant_id, {
                  wallet_id: walletID.trim(),
                  ufvk: ufvk.trim(),
                  chain: chain.trim(),
                  ua_hrp: uaHRP.trim(),
                  coin_type: coinType,
                });
                setWallet(out);
                await refresh(merchant.merchant_id);
              } catch (e) {
                if (e instanceof APIError && e.code === "conflict") {
                  await refresh(merchant.merchant_id);
                  setError("wallet already set");
                  return;
                }
                setError(e instanceof Error ? e.message : "wallet set failed");
              }
            }}
          >
            <div className="sm:col-span-2">
              <label className="block text-sm font-medium text-zinc-700">UFVK</label>
              <textarea
                value={ufvk}
                onChange={(e) => setUfVK(e.target.value)}
                className="mt-1 h-28 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                placeholder="jview1..."
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-zinc-700">Wallet ID (optional)</label>
              <input
                value={walletID}
                onChange={(e) => setWalletID(e.target.value)}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                placeholder={`wallet_${merchant.merchant_id}`}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-zinc-700">Chain</label>
              <input value={chain} onChange={(e) => setChain(e.target.value)} className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm" />
            </div>
            <div>
              <label className="block text-sm font-medium text-zinc-700">UA HRP</label>
              <input value={uaHRP} onChange={(e) => setUAHRP(e.target.value)} className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm" />
            </div>
            <div>
              <label className="block text-sm font-medium text-zinc-700">Coin Type</label>
              <input
                type="number"
                min={0}
                value={coinType}
                onChange={(e) => setCoinType(Number.parseInt(e.target.value || "0", 10))}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              />
            </div>
            <div className="sm:col-span-2">
              <button type="submit" className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800">
                Set Wallet
              </button>
            </div>
          </form>
        )}
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Merchant API Keys</h2>
        <p className="mt-1 text-sm text-zinc-600">API keys are shown once at creation time.</p>

        <div className="mt-4">
          <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">Existing Keys</div>
          {apiKeys.length === 0 ? (
            <div className="mt-2 text-sm text-zinc-600">No keys.</div>
          ) : (
            <div className="mt-2 overflow-x-auto">
              <table className="min-w-full border-separate border-spacing-0">
                <thead>
                  <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                    <th className="border-b border-zinc-200 px-3 py-2">Key ID</th>
                    <th className="border-b border-zinc-200 px-3 py-2">Label</th>
                    <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                    <th className="border-b border-zinc-200 px-3 py-2">Created</th>
                    <th className="border-b border-zinc-200 px-3 py-2">Revoked</th>
                  </tr>
                </thead>
                <tbody>
                  {apiKeys.map((k) => (
                    <tr key={k.key_id} className="text-sm text-zinc-950">
                      <td className="border-b border-zinc-100 px-3 py-2">
                        <div className="font-mono text-xs">{k.key_id}</div>
                      </td>
                      <td className="border-b border-zinc-100 px-3 py-2">{k.label || <span className="text-xs text-zinc-500">—</span>}</td>
                      <td className="border-b border-zinc-100 px-3 py-2">{k.revoked_at ? "revoked" : "active"}</td>
                      <td className="border-b border-zinc-100 px-3 py-2">
                        <div className="text-xs">{k.created_at}</div>
                      </td>
                      <td className="border-b border-zinc-100 px-3 py-2">
                        <div className="text-xs">{k.revoked_at ?? "—"}</div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <form
            className="space-y-3"
            onSubmit={async (e) => {
              e.preventDefault();
              setError(null);
              setCreatedKey(null);
              try {
                const out = await createAPIKey(merchant.merchant_id, apiKeyLabel.trim() || "default");
                setCreatedKey({ api_key: out.api_key, key_id: out.key.key_id });
                await refresh(merchant.merchant_id);
              } catch (e) {
                setError(e instanceof Error ? e.message : "api key create failed");
              }
            }}
          >
            <div>
              <label className="block text-sm font-medium text-zinc-700">New Key Label</label>
              <input
                value={apiKeyLabel}
                onChange={(e) => setAPIKeyLabel(e.target.value)}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              />
            </div>
            <button type="submit" className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800">
              Create API Key
            </button>

            {createdKey ? (
              <div className="rounded-md border border-zinc-200 bg-zinc-50 p-3">
                <div className="text-xs font-semibold uppercase tracking-wider text-zinc-500">Key ID</div>
                <div className="mt-1 font-mono text-xs">{createdKey.key_id}</div>
                <div className="mt-3 text-xs font-semibold uppercase tracking-wider text-zinc-500">API Key</div>
                <div className="mt-1 font-mono text-xs break-all">{createdKey.api_key}</div>
              </div>
            ) : null}
          </form>

          <form
            className="space-y-3"
            onSubmit={async (e) => {
              e.preventDefault();
              const v = revokeKeyID.trim();
              if (!v) return;
              setError(null);
              try {
                await revokeAPIKey(v);
                setRevokeKeyID("");
                await refresh(merchant.merchant_id);
              } catch (e) {
                setError(e instanceof Error ? e.message : "revoke failed");
              }
            }}
          >
            <div>
              <label className="block text-sm font-medium text-zinc-700">Revoke Key ID</label>
              <input
                value={revokeKeyID}
                onChange={(e) => setRevokeKeyID(e.target.value)}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                placeholder="key_..."
              />
            </div>
            <button type="submit" className="rounded-md border border-zinc-200 bg-white px-4 py-2 text-sm font-medium text-zinc-950 hover:bg-zinc-50">
              Revoke
            </button>
          </form>
        </div>

        {error ? (
          <div className="mt-4">
            <ErrorBanner message={error} />
          </div>
        ) : null}
      </section>
    </div>
  );
}
