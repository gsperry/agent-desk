import { defineConfig, devices } from '@playwright/test'

export const port = 7420
export const token = 'e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0e2e0'

// The smoke test runs the built binary against fixture sessions, so it
// needs neither agent-manager nor tmux. Run `make build` first. Fixtures are
// copied first because a send appends to the fixture transcript.
export default defineConfig({
  testDir: 'e2e',
  timeout: 30_000,
  use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
  webServer: {
    command: `rm -rf .e2e-fixtures && cp -RL ../testdata/fixtures/desk .e2e-fixtures && ../bin/agent-desk --no-open --port ${String(port)} --fixtures .e2e-fixtures`,
    url: `http://127.0.0.1:${String(port)}/`,
    env: { AGENT_DESK_TOKEN: token },
    reuseExistingServer: false,
  },
})
