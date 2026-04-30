import React from "react";
import { describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";

const replaceMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: replaceMock }),
  usePathname: () => "/status",
}));

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return { ...actual, getAdminStatus: vi.fn() };
});

import { AdminProvider, useAdmin } from "@/components/AdminProvider";
import { APIError, getAdminStatus } from "@/lib/api";

function Probe() {
  const admin = useAdmin();
  return (
    <div>
      <div data-testid="loading">{String(admin.loading)}</div>
      <div data-testid="error">{admin.error ?? ""}</div>
      <div data-testid="bestHeight">{admin.status?.chain.best_height ?? ""}</div>
    </div>
  );
}

it("redirects to /login on 401", async () => {
  replaceMock.mockReset();
  vi.mocked(getAdminStatus).mockRejectedValueOnce(new APIError("unauthorized", 401));
  render(
    <AdminProvider>
      <Probe />
    </AdminProvider>,
  );
  await waitFor(() => expect(replaceMock).toHaveBeenCalledWith("/login"));
});

it("provides status on success", async () => {
  replaceMock.mockReset();
  vi.mocked(getAdminStatus).mockResolvedValueOnce({
    chain: { best_height: 123, best_hash: "00", uptime_seconds: 1 },
    scanner: { connected: true, last_cursor_applied: 1, last_event_at: null },
    event_delivery: { pending_deliveries: 0 },
    restarts: { junocashd_restarts_detected: 0, last_restart_at: null },
  });

  const { getByTestId } = render(
    <AdminProvider>
      <Probe />
    </AdminProvider>,
  );

  await waitFor(() => expect(getByTestId("bestHeight")).toHaveTextContent("123"));
  expect(replaceMock).not.toHaveBeenCalled();
});
