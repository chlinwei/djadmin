import { defineConfig, devices } from '@playwright/test'

const PORT = Number(process.env.PW_PORT || 5173)
const HOST = process.env.PW_HOST || '127.0.0.1'
const BASE_URL = process.env.PW_BASE_URL || `http://${HOST}:${PORT}`
const API_BASE_URL = process.env.PW_API_BASE_URL || 'http://127.0.0.1:19000'
const HEADED = ['1', 'true', 'yes', 'on'].includes(String(process.env.PW_HEADED || '').toLowerCase())
const toBool = (value) => ['1', 'true', 'yes', 'on'].includes(String(value || '').toLowerCase())
const isWindows = process.platform === 'win32'
const SKIP_BACKEND_SERVER = toBool(process.env.PW_SKIP_BACKEND_SERVER) || isWindows
const SKIP_FRONTEND_SERVER = toBool(process.env.PW_SKIP_FRONTEND_SERVER)

const webServers = []

if (!SKIP_BACKEND_SERVER) {
  webServers.push({
    command: 'bash -lc "cd ../backend/djadmin && bash scripts/start_e2e_server.sh"',
    url: API_BASE_URL,
    reuseExistingServer: true,
    timeout: 180000,
  })
}

if (!SKIP_FRONTEND_SERVER) {
  const frontendCommand = isWindows
    ? `cmd /c "set VITE_API_BASE_URL=${API_BASE_URL}&& npm run dev -- --host ${HOST} --port ${PORT}"`
    : `bash -lc "VITE_API_BASE_URL=${API_BASE_URL} npm run dev -- --host ${HOST} --port ${PORT}"`

  webServers.push({
    command: frontendCommand,
    url: BASE_URL,
    reuseExistingServer: true,
    timeout: 120000,
  })
}

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['html', { open: 'never' }], ['list']] : 'list',
  use: {
    baseURL: BASE_URL,
    headless: !HEADED,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'setup',
      testMatch: /auth\.setup\.spec\.js/,
    },
    {
      name: 'chromium',
      testIgnore: /auth\.setup\.spec\.js/,
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'playwright/.auth/user.json',
      },
      dependencies: ['setup'],
    },
  ],
  webServer: webServers,
})
