export function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b th-border py-2">
      <div className="text-sm th-dim">{label}</div>
      <div className="text-sm font-medium th-text">{value}</div>
    </div>
  );
}
