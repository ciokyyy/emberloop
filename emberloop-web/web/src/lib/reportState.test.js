import { describe, it, expect, beforeEach } from 'vitest';
import { isReportUnread, markReportsRead, readReportState, receiveReport, rememberReports, setReportDismissed, writeReportState } from './reportState.js';

beforeEach(() => localStorage.clear());

describe('per-User, per-Session report state', () => {
  it('persists dismissed reports and restores them after reload', () => {
    const report = { seq: 12, groupName: 'group-a', body: 'result', members: [] };
    let state = rememberReports(readReportState('user-a', 'session-a'), [report]);
    state = setReportDismissed(state, 12, true);
    writeReportState('user-a', 'session-a', state);

    const reloaded = readReportState('user-a', 'session-a');
    expect(reloaded.reports).toEqual([report]);
    expect(reloaded.dismissed).toEqual([12]);
    expect(isReportUnread('user-a', 'session-a', 12)).toBe(true);
    expect(isReportUnread('user-b', 'session-a', 12)).toBe(true);
    expect(isReportUnread('user-a', 'session-b', 12)).toBe(true);

    writeReportState('user-a', 'session-a', setReportDismissed(reloaded, 12, false));
    expect(readReportState('user-a', 'session-a').dismissed).toEqual([]);
  });

  it('keeps unread until opening the Session records its latest report sequence', () => {
    const state = rememberReports(readReportState('user-a', 'session-a'), [{ seq: 18, groupName: 'group-b' }]);
    writeReportState('user-a', 'session-a', state);
    expect(isReportUnread('user-a', 'session-a')).toBe(true);
    writeReportState('user-a', 'session-a', markReportsRead(state, 18));
    expect(isReportUnread('user-a', 'session-a')).toBe(false);
  });

  it('does not leave a report unread when it arrives in the open Session', () => {
    const report = { seq: 22, groupName: 'group-c' };
    const background = receiveReport(readReportState('user-a', 'session-a'), report);
    writeReportState('user-a', 'session-a', background);
    expect(isReportUnread('user-a', 'session-a')).toBe(true);

    const open = receiveReport(readReportState('user-a', 'session-a'), { ...report, seq: 23 }, true);
    writeReportState('user-a', 'session-a', open);
    expect(isReportUnread('user-a', 'session-a')).toBe(false);
    expect(open.reports.map((item) => item.seq)).toEqual([22, 23]);
  });
});
