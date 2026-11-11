import { defineConfig, devices } from '@playwright/test'

export const port = 7420
export const token = 'e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0'

// The smoke test runs the built binary against fixture sessions, so it
// needs neither agent-manager nor tmux. Run `make build` first.
export default defineConfig({
  testDir: 'e2e',
  timeout: 30_000,
  use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
  webServer: {
    command: `../bin/agent-desk --no-open --port ${String(port)} --fixtures ../testdata/fixtures/desk`,
    url: `http://127.0.0.1:${String(port)}/`,
    env: { AGENT_DESK_TOKEN: token },
    reuseExistingServer: false,
  },
})
