// No-op teardown for local-server mode — we don't own the server, so don't kill it.
export default async function globalTeardown() {
  console.log('[E2E-local] Skipping teardown (local server not managed by test runner).');
}
