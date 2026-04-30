import { defineConfig } from "@playwright/test";

const frontPort = Number.parseInt(process.env.E2E_FRONTEND_PORT ?? "39081", 10);

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  use: {
    baseURL: `http://127.0.0.1:${frontPort}`,
  },
  webServer: {
    command: "node e2e/serve.js",
    url: `http://127.0.0.1:${frontPort}/admin/`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
