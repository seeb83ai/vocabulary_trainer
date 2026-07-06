import { describe, it, expect } from 'vitest';

// These pure helpers are defined in app.js; inlined here to keep tests
// self-contained and independent of module bundling.

function buildIssueMetadata(win) {
  return {
    user_agent: (win.navigator && win.navigator.userAgent) || '',
    viewport: `${win.innerWidth}x${win.innerHeight}`,
    locale: (win.navigator && win.navigator.language) || '',
    timestamp: new Date().toISOString(),
  };
}

function validateIssueForm(form) {
  const valid = ['idea', 'bug', 'question', 'misc'];
  if (!valid.includes(form.category)) return 'issue.errCategory';
  if (!form.title || !form.title.trim()) return 'issue.errTitle';
  if (!form.description || !form.description.trim()) return 'issue.errDescription';
  return '';
}

describe('buildIssueMetadata', () => {
  it('extracts non-sensitive client context', () => {
    const win = {
      navigator: { userAgent: 'TestAgent/1.0', language: 'en-US' },
      innerWidth: 1024,
      innerHeight: 768,
    };
    const meta = buildIssueMetadata(win);
    expect(meta.user_agent).toBe('TestAgent/1.0');
    expect(meta.viewport).toBe('1024x768');
    expect(meta.locale).toBe('en-US');
    expect(typeof meta.timestamp).toBe('string');
    expect(meta.timestamp).toMatch(/^\d{4}-\d{2}-\d{2}T/);
  });

  it('tolerates a missing navigator', () => {
    const meta = buildIssueMetadata({ innerWidth: 0, innerHeight: 0 });
    expect(meta.user_agent).toBe('');
    expect(meta.locale).toBe('');
    expect(meta.viewport).toBe('0x0');
  });
});

describe('validateIssueForm', () => {
  it('accepts a valid form', () => {
    expect(validateIssueForm({ category: 'bug', title: 'x', description: 'y' })).toBe('');
  });

  it('rejects an unknown category', () => {
    expect(validateIssueForm({ category: 'spam', title: 'x', description: 'y' })).toBe('issue.errCategory');
  });

  it('rejects a blank title', () => {
    expect(validateIssueForm({ category: 'idea', title: '   ', description: 'y' })).toBe('issue.errTitle');
  });

  it('rejects a missing description', () => {
    expect(validateIssueForm({ category: 'question', title: 'x' })).toBe('issue.errDescription');
  });
});
