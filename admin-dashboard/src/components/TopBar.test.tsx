import React from "react";
import { expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render } from "@testing-library/react";

const pushMock = vi.fn();
const replaceMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, replace: replaceMock }),
}));

vi.mock("@/components/AdminProvider", () => ({
  useAdmin: () => ({ refreshStatus: vi.fn() }),
}));

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return { ...actual, adminLogout: vi.fn() };
});

import { TopBar } from "@/components/TopBar";

beforeEach(() => {
  pushMock.mockReset();
  replaceMock.mockReset();
});

it("routes inv_ to invoice detail", async () => {
  const { getByPlaceholderText, getByRole } = render(<TopBar />);
  fireEvent.change(getByPlaceholderText("Search inv_, m_, txid, or external_order_id"), { target: { value: "inv_123" } });
  fireEvent.click(getByRole("button", { name: "Go" }));
  expect(pushMock).toHaveBeenCalledWith("/invoice?invoice_id=inv_123");
});

it("routes m_ to merchant detail", async () => {
  const { getByPlaceholderText, getByRole } = render(<TopBar />);
  fireEvent.change(getByPlaceholderText("Search inv_, m_, txid, or external_order_id"), { target: { value: "m_abc" } });
  fireEvent.click(getByRole("button", { name: "Go" }));
  expect(pushMock).toHaveBeenCalledWith("/merchant?merchant_id=m_abc");
});

it("routes txid to deposits filter", async () => {
  const { getByPlaceholderText, getByRole } = render(<TopBar />);
  const txid = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  fireEvent.change(getByPlaceholderText("Search inv_, m_, txid, or external_order_id"), { target: { value: txid } });
  fireEvent.click(getByRole("button", { name: "Go" }));
  expect(pushMock).toHaveBeenCalledWith(`/deposits?txid=${txid}`);
});

it("routes other queries to invoices external_order_id filter", async () => {
  const { getByPlaceholderText, getByRole } = render(<TopBar />);
  fireEvent.change(getByPlaceholderText("Search inv_, m_, txid, or external_order_id"), { target: { value: "order-1" } });
  fireEvent.click(getByRole("button", { name: "Go" }));
  expect(pushMock).toHaveBeenCalledWith("/invoices?external_order_id=order-1");
});

it("does not route on empty query", async () => {
  const { getByRole } = render(<TopBar />);
  fireEvent.click(getByRole("button", { name: "Go" }));
  expect(pushMock).not.toHaveBeenCalled();
});

