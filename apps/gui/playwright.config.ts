import { defineConfig, devices } from "@playwright/test";

const guiPort = process.env.ARTEMIS_GUI_PORT ?? "5173";
const baseURL = process.env.ARTEMIS_GUI_URL ?? `http://127.0.0.1:${guiPort}`;
const controlPlaneUrl = process.env.VITE_CONTROL_PLANE_URL ?? "http://127.0.0.1:8000";

export default defineConfig({
  testDir: "./tests",
  timeout: 45_000,
  workers: 1,
  expect: {
    timeout: 15_000
  },
  use: {
    baseURL,
    trace: "retain-on-failure"
  },
  webServer: {
    command: `npm run dev -- --port ${guiPort}`,
    url: baseURL,
    reuseExistingServer: true,
    env: {
      ...process.env,
      VITE_CONTROL_PLANE_URL: controlPlaneUrl
    },
    timeout: 30_000
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] }
    }
  ]
});
