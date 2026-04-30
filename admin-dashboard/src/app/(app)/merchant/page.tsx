"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
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
import { inputCls, selectCls } from "@/lib/styles";

function cloneSettings(s: MerchantSettings): MerchantSettings {
  return structuredClone(s);
}

export default function MerchantDetailPage() {
  const router = useRouter();
  const [merchantID, setMerchantID] = useState("");

  const [merchant, setMerchant] = useState<Merchant | null>(null);
  const [settings, setSettings] = useState<MerchantSettings | null>(null);
  const [wallet, setWallet] = useState<MerchantWallet | null>(null);
  const [apiKeys, setAPIKeys] = useState<MerchantAPIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [walletID, setWalletID] = useState("");
  const [ufvk, setUfVK] = useState("");
  const [chain, setChain] = useState("mainnet");
  const [uaHRP, setUAHRP] = useState("j");
  const [coinType, setCoinType] = useState(8133);

  const [apiKeyLabel, setAPIKeyLabel] = useState("default");
  const [createdKey, setCreatedKey] = useState<{ api_key: string; key_id: string } | null>(null);
  const [revokeKeyID, setRevokeKeyID] = useState("");
  const [savingSettings, setSavingSettings] = useState(false);
  const [settingWallet, setSettingWallet] = useState(false);
  const [creatingKey, setCreatingKey] = useState(false);
  const [revokingKey, setRevokingKey] = useState(false);

  async function refresh(id: string) {
    const v = id.trim();
    if (!v) return;
    try {
      setRefreshing(true);
      setError(null);
      const m = await getMerchant(v);
      setMerchant(m);
      setSettings(cloneSettings(m.settings));
      setWallet(m.wallet ?? null);
      setAPIKeys(m.api_keys ?? []);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) { router.replace("/login"); return; }
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("merchant_id") ?? "";
    setMerchantID(id);
    void refresh(id);
  }, []);

  if (!merchantID) {
    return <div className="text-sm th-dim">merchant_id is required.</div>;
  }
  if (loading && !merchant) {
    return <div className="text-sm th-dim">Loading...</div>;
  }
  if (error) {
    return <ErrorBanner message={error} />;
  }
  if (!merchant || !settings) {
    return <div className="text-sm th-dim">Merchant not found.</div>;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">{merchant.name}</h1>
        <div className="mt-1 font-mono text-xs th-faint">{merchant.merchant_id}</div>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Settings</h2>
          <button
            type="button"
            onClick={() => refresh(merchant.merchant_id)}
            disabled={refreshing}
            className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-xs transition-colors disabled:opacity-60"
          >
            {refreshing ? "Refreshing..." : "Refresh"}
          </button>
        </div>

        <form
          className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
          onSubmit={async (e) => {
            e.preventDefault();
            setError(null);
            try {
              setSavingSettings(true);
              const updated = await setMerchantSettings(merchant.merchant_id, settings);
              setMerchant(updated);
              setSettings(cloneSettings(updated.settings));
            } catch (e) {
              setError(e instanceof Error ? e.message : "save failed");
            } finally {
              setSavingSettings(false);
            }
          }}
        >
          <div>
            <label className="block text-xs th-muted mb-1">Invoice TTL (seconds)</label>
            <input
              type="number"
              min={0}
              value={settings.invoice_ttl_seconds}
              onChange={(e) => setSettings({ ...settings, invoice_ttl_seconds: Number.parseInt(e.target.value || "0", 10) })}
              className={inputCls}
            />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Required Confirmations</label>
            <input
              type="number"
              min={0}
              value={settings.required_confirmations}
              onChange={(e) => setSettings({ ...settings, required_confirmations: Number.parseInt(e.target.value || "0", 10) })}
              className={inputCls}
            />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Late Payment Policy</label>
            <select
              value={settings.policies.late_payment_policy}
              onChange={(e) => setSettings({ ...settings, policies: { ...settings.policies, late_payment_policy: e.target.value } })}
              className={selectCls}
            >
              <option value="mark_paid_late">mark_paid_late</option>
              <option value="manual_review">manual_review</option>
              <option value="ignore">ignore</option>
            </select>
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Partial Payment Policy</label>
            <select
              value={settings.policies.partial_payment_policy}
              onChange={(e) => setSettings({ ...settings, policies: { ...settings.policies, partial_payment_policy: e.target.value } })}
              className={selectCls}
            >
              <option value="accept_partial">accept_partial</option>
              <option value="reject_partial">reject_partial</option>
            </select>
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Overpayment Policy</label>
            <select
              value={settings.policies.overpayment_policy}
              onChange={(e) => setSettings({ ...settings, policies: { ...settings.policies, overpayment_policy: e.target.value } })}
              className={selectCls}
            >
              <option value="mark_overpaid">mark_overpaid</option>
              <option value="manual_review">manual_review</option>
            </select>
          </div>

          <div className="sm:col-span-2">
            <button
              type="submit"
              disabled={savingSettings}
              className="btn-gold rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              {savingSettings ? "Saving..." : "Save Settings"}
            </button>
          </div>
        </form>
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Wallet (UFVK)</h2>
        <p className="mt-1 text-xs th-dim">Immutable once set.</p>

        {wallet ? (
          <div className="mt-4 rounded-xl border th-border th-input p-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">Wallet ID</div>
                <div className="mt-1 font-mono text-xs th-dim">{wallet.wallet_id}</div>
              </div>
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">Created At</div>
                <div className="mt-1 text-xs th-dim">{wallet.created_at}</div>
              </div>
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">Chain</div>
                <div className="mt-1 text-xs th-dim">{wallet.chain}</div>
              </div>
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">UA HRP</div>
                <div className="mt-1 text-xs th-dim">{wallet.ua_hrp}</div>
              </div>
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">Coin Type</div>
                <div className="mt-1 text-xs th-dim">{wallet.coin_type}</div>
              </div>
            </div>

            <details className="mt-3">
              <summary className="cursor-pointer text-sm th-muted">UFVK</summary>
              <div className="mt-2 font-mono text-xs break-all th-dim">{wallet.ufvk}</div>
            </details>
          </div>
        ) : (
          <form
            className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
            onSubmit={async (e) => {
              e.preventDefault();
              setError(null);
              try {
                setSettingWallet(true);
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
              } finally {
                setSettingWallet(false);
              }
            }}
          >
            <div className="sm:col-span-2">
              <label className="block text-xs th-muted mb-1">UFVK</label>
              <textarea
                value={ufvk}
                onChange={(e) => setUfVK(e.target.value)}
                className="h-28 w-full rounded-lg border th-border th-input th-text px-3 py-2 text-sm focus:outline-none focus:border-[#dc8548]/40"
                placeholder="jview1..."
                required
              />
            </div>
            <div>
              <label className="block text-xs th-muted mb-1">Wallet ID (optional)</label>
              <input
                value={walletID}
                onChange={(e) => setWalletID(e.target.value)}
                className={inputCls}
                placeholder={`wallet_${merchant.merchant_id}`}
              />
            </div>
            <div>
              <label className="block text-xs th-muted mb-1">Chain</label>
              <input value={chain} onChange={(e) => setChain(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block text-xs th-muted mb-1">UA HRP</label>
              <input value={uaHRP} onChange={(e) => setUAHRP(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block text-xs th-muted mb-1">Coin Type</label>
              <input
                type="number"
                min={0}
                value={coinType}
                onChange={(e) => setCoinType(Number.parseInt(e.target.value || "0", 10))}
                className={inputCls}
              />
            </div>
            <div className="sm:col-span-2">
              <button
                type="submit"
                disabled={settingWallet}
                className="btn-gold rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
              >
                {settingWallet ? "Setting..." : "Set Wallet"}
              </button>
            </div>
          </form>
        )}
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Merchant API Keys</h2>
        <p className="mt-1 text-xs th-dim">API keys are shown once at creation time.</p>

        <div className="mt-4">
          <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">Existing Keys</div>
          {apiKeys.length === 0 ? (
            <div className="mt-2 text-sm th-dim">No keys.</div>
          ) : (
            <div className="mt-2 overflow-x-auto">
              <table className="min-w-full border-separate border-spacing-0">
                <thead>
                  <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                    <th className="border-b th-border px-3 py-2">Key ID</th>
                    <th className="border-b th-border px-3 py-2">Label</th>
                    <th className="border-b th-border px-3 py-2">Status</th>
                    <th className="border-b th-border px-3 py-2">Created</th>
                    <th className="border-b th-border px-3 py-2">Revoked</th>
                  </tr>
                </thead>
                <tbody>
                  {apiKeys.map((k) => (
                    <tr key={k.key_id} className="text-sm th-text">
                      <td className="border-b th-border px-3 py-2">
                        <div className="font-mono text-xs th-dim">{k.key_id}</div>
                      </td>
                      <td className="border-b th-border px-3 py-2 th-dim">{k.label || <span className="text-xs th-faint">—</span>}</td>
                      <td className="border-b th-border px-3 py-2 th-dim">{k.revoked_at ? "revoked" : "active"}</td>
                      <td className="border-b th-border px-3 py-2">
                        <div className="text-xs th-dim">{k.created_at}</div>
                      </td>
                      <td className="border-b th-border px-3 py-2">
                        <div className="text-xs th-dim">{k.revoked_at ?? "—"}</div>
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
                setCreatingKey(true);
                const out = await createAPIKey(merchant.merchant_id, apiKeyLabel.trim() || "default");
                setCreatedKey({ api_key: out.api_key, key_id: out.key.key_id });
                await refresh(merchant.merchant_id);
              } catch (e) {
                setError(e instanceof Error ? e.message : "api key create failed");
              } finally {
                setCreatingKey(false);
              }
            }}
          >
            <div>
              <label className="block text-xs th-muted mb-1">New Key Label</label>
              <input
                value={apiKeyLabel}
                onChange={(e) => setAPIKeyLabel(e.target.value)}
                className={inputCls}
              />
            </div>
            <button type="submit" disabled={creatingKey} className="btn-gold rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-60">
              {creatingKey ? "Creating..." : "Create API Key"}
            </button>

            {createdKey ? (
              <div className="rounded-xl border th-border th-input p-4">
                <div className="text-[10px] font-semibold uppercase tracking-wider th-faint">Key ID</div>
                <div className="mt-1 font-mono text-xs th-dim">{createdKey.key_id}</div>
                <div className="mt-3 text-[10px] font-semibold uppercase tracking-wider th-faint">API Key</div>
                <div className="mt-1 font-mono text-xs break-all th-dim">{createdKey.api_key}</div>
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
                setRevokingKey(true);
                await revokeAPIKey(v);
                setRevokeKeyID("");
                await refresh(merchant.merchant_id);
              } catch (e) {
                setError(e instanceof Error ? e.message : "revoke failed");
              } finally {
                setRevokingKey(false);
              }
            }}
          >
            <div>
              <label className="block text-xs th-muted mb-1">Revoke Key ID</label>
              <input
                value={revokeKeyID}
                onChange={(e) => setRevokeKeyID(e.target.value)}
                className={inputCls}
                placeholder="key_..."
              />
            </div>
            <button
              type="submit"
              disabled={revokingKey}
              className="rounded-lg border th-border th-hover th-muted px-4 py-2 text-sm font-medium transition-colors disabled:opacity-60"
            >
              {revokingKey ? "Revoking..." : "Revoke"}
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
