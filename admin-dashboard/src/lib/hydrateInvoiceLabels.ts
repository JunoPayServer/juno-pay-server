import { getInvoice } from "@/lib/api";

export async function hydrateInvoiceLabels(
  ids: (string | null | undefined)[],
  existing: Record<string, string>,
  setLabels: (updater: (prev: Record<string, string>) => Record<string, string>) => void,
): Promise<void> {
  const unique = Array.from(new Set(ids.filter((v): v is string => Boolean(v?.trim()))));
  const missing = unique.filter((id) => !existing[id]);
  if (missing.length === 0) return;

  const entries = await Promise.all(
    missing.map(async (id) => {
      try {
        const inv = await getInvoice(id);
        return [id, inv.external_order_id] as const;
      } catch {
        return null;
      }
    }),
  );

  const next: Record<string, string> = {};
  for (const e of entries) {
    if (e) next[e[0]] = e[1];
  }
  if (Object.keys(next).length === 0) return;
  setLabels((prev) => ({ ...prev, ...next }));
}
