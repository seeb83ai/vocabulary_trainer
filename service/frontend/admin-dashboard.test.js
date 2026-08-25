import { describe, it, expect } from 'vitest';

// ── buildStatTiles ───────────────────────────────────────────────────────────
// Inline from admin-dashboard.js to test in isolation.

function buildStatTiles(ov) {
  return [
    { label: 'Total users', value: String(ov.users.total) },
    { label: 'Verified / Unverified', value: `${ov.users.verified} / ${ov.users.unverified}` },
    { label: 'Admin / Plus / Free', value: `${ov.users.admins} / ${ov.users.plus} / ${ov.users.free}` },
    { label: 'Active (7d)', value: String(ov.activity.active_last_7_days) },
    { label: 'Active (30d)', value: String(ov.activity.active_last_30_days) },
    { label: 'Dormant', value: String(ov.activity.dormant) },
  ];
}

function overviewFixture(overrides = {}) {
  return {
    users: { total: 5, admins: 1, plus: 2, free: 2, verified: 4, unverified: 1, ...overrides.users },
    activity: { active_last_7_days: 3, active_last_30_days: 4, dormant: 1, ...overrides.activity },
  };
}

describe('buildStatTiles', () => {
  it('builds six tiles in a fixed order', () => {
    const tiles = buildStatTiles(overviewFixture());
    expect(tiles).toHaveLength(6);
    expect(tiles.map(t => t.label)).toEqual([
      'Total users',
      'Verified / Unverified',
      'Admin / Plus / Free',
      'Active (7d)',
      'Active (30d)',
      'Dormant',
    ]);
  });

  it('formats the total users tile', () => {
    const tiles = buildStatTiles(overviewFixture());
    expect(tiles[0]).toEqual({ label: 'Total users', value: '5' });
  });

  it('formats the verified/unverified tile as a ratio', () => {
    const tiles = buildStatTiles(overviewFixture());
    expect(tiles[1]).toEqual({ label: 'Verified / Unverified', value: '4 / 1' });
  });

  it('formats the role breakdown tile', () => {
    const tiles = buildStatTiles(overviewFixture());
    expect(tiles[2]).toEqual({ label: 'Admin / Plus / Free', value: '1 / 2 / 2' });
  });

  it('formats activity tiles', () => {
    const tiles = buildStatTiles(overviewFixture());
    expect(tiles[3]).toEqual({ label: 'Active (7d)', value: '3' });
    expect(tiles[4]).toEqual({ label: 'Active (30d)', value: '4' });
    expect(tiles[5]).toEqual({ label: 'Dormant', value: '1' });
  });

  it('handles all-zero stats', () => {
    const tiles = buildStatTiles(overviewFixture({
      users: { total: 0, admins: 0, plus: 0, free: 0, verified: 0, unverified: 0 },
      activity: { active_last_7_days: 0, active_last_30_days: 0, dormant: 0 },
    }));
    expect(tiles[0].value).toBe('0');
    expect(tiles[1].value).toBe('0 / 0');
  });
});
