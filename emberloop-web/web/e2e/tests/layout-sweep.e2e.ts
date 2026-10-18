import { test } from '@e2e-dev/web';
import { expect } from 'e2e';
import { SETTINGS_PAGE_IDS, SETTINGS_PAGES } from '../../src/lib/settings.js';
import { parsePath } from '../../src/lib/router.js';
import { adminCookie } from '../support/auth.ts';

const SIZES = [
  { name: 'phone', width: 390, height: 844 },
  { name: 'desktop', width: 1280, height: 800 },
];
const REPO = '/tmp/el-e2e/repo';
const MISSING_RUN = 'layout-probe-missing-run';
const MISSING_TICKET = 'layout-probe-missing-ticket';

async function api(app: { baseUrl: string | undefined }, path: string, body?: unknown, cookie?: string) {
  const res = await fetch(`${app.baseUrl}${path}`, {
    method: body ? 'POST' : 'GET',
    headers: { 'Content-Type': 'application/json', Cookie: cookie ?? (await adminCookie(app.baseUrl)) },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${path} failed: ${res.status} ${await res.text()}`);
  return res.json();
}

async function waitForTurn(app: { baseUrl: string | undefined }, id: string, seq: number, cookie: string) {
  for (let attempt = 0; attempt < 80; attempt++) {
    const view = await api(app, `/api/sessions/${id}`, undefined, cookie);
    if (view.session?.status === 'idle' && view.events?.some((event: { seq: number; kind: string }) => event.seq > seq && event.kind === 'assistant_chunk')) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error(`Session ${id} did not finish the layout fixture turn`);
}

function routes(projectId: string, sessionId: string, seq: number) {
  return [
    { path: '/', route: 'home', heading: 'Dashboard' },
    { path: '/p/none', route: 'project' },
    { path: `/p/${projectId}`, route: 'project' },
    { path: `/p/${projectId}/settings`, route: 'project-settings', heading: 'Project settings' },
    { path: `/p/${projectId}/runs`, route: 'project-runs', heading: 'Runs' },
    { path: `/p/${projectId}/runs/${MISSING_RUN}`, route: 'project-run', heading: 'Runs' },
    { path: `/p/${projectId}/runs/${MISSING_RUN}/${MISSING_TICKET}`, route: 'project-ticket', heading: `Ticket ${MISSING_TICKET}` },
    { path: `/s/${sessionId}?at=${seq}`, route: 'session' },
    { path: '/runs', route: 'runs', heading: 'Runs' },
    { path: `/runs/${MISSING_RUN}`, route: 'run', heading: 'Runs' },
    { path: `/runs/${MISSING_RUN}/${MISSING_TICKET}`, route: 'ticket', heading: `Ticket ${MISSING_TICKET}` },
    { path: '/settings', route: 'settings', heading: 'Settings', desktopPath: '/settings/agents', desktopHeading: 'Agents' },
    ...SETTINGS_PAGE_IDS.map((section) => ({
      path: `/settings/${section}`,
      route: 'settings',
      heading: SETTINGS_PAGES[section].label,
    })),
    { path: '/quota', route: 'quota', heading: 'Quota' },
    { path: '/providers', route: 'providers', heading: 'Providers' },
    { path: '/memory', route: 'memory', heading: 'Memory' },
  ];
}

async function visit(browser: { evaluate(script: string): Promise<unknown> }, path: string) {
  await browser.evaluate(`(() => {
    history.pushState(null, '', ${JSON.stringify(path)});
    window.dispatchEvent(new PopStateEvent('popstate', { state: history.state }));
  })()`);
}

async function measure(browser: { evaluate(script: () => unknown): Promise<any> }) {
  await browser.evaluate(async () => {
    const finiteAnimations = document.getAnimations().filter((animation) => animation.effect?.getTiming().iterations !== Infinity);
    await Promise.all(finiteAnimations.map((animation) => animation.finished.catch(() => undefined)));
  });
  return browser.evaluate(() => {
    const main = document.querySelector('main');
    const pageRoots = [...document.querySelectorAll<HTMLElement>('main [data-testid$="-page"], main [data-testid="dashboard"]')];
    const scrollerWidths = pageRoots.map((root) => {
      const scroller = root.firstElementChild as HTMLElement | null;
      return scroller ? scroller.scrollWidth - scroller.clientWidth : 0;
    });
    const controls = [...document.querySelectorAll<HTMLElement>(
      'main button, main a[href], main [role="button"], main input:not([type="hidden"]), main [role="switch"]',
    )].filter((element) => element.getClientRects().length > 0 && getComputedStyle(element).display !== 'none');
    const hoverOnlyControls = controls.filter((element) => {
      const classes = [];
      for (let parent: HTMLElement | null = element; parent; parent = parent.parentElement) {
        classes.push(...parent.className.toString().split(/\s+/));
        if (parent.tagName === 'MAIN') break;
      }
      const hoverGate = classes.some((name) => /(?:group-)?hover:.*(?:opacity-|visible|invisible|hidden|block|flex|grid|pointer-events-)/.test(name));
      const keyboardGate = classes.some((name) => /(?:group-)?focus(?:-within|-visible)?:.*(?:opacity-|visible|invisible|hidden|block|flex|grid|pointer-events-)/.test(name));
      return hoverGate && !keyboardGate;
    });
    const primaryTargets = controls.filter((element) => element.classList.contains('bg-primary') || element.dataset.primaryTarget === 'true');
    const smallTargets = primaryTargets
      .map((element) => ({
        label: element.getAttribute('aria-label') || element.textContent?.trim().replace(/\s+/g, ' ').slice(0, 48) || element.tagName,
        height: Math.round(element.getBoundingClientRect().height),
      }))
      .filter((target) => target.height > 0 && target.height < 44);
    const header = main?.querySelectorAll('[data-page-header]').length ?? 0;
    const h1 = main?.querySelectorAll('h1').length ?? 0;
    return {
      path: window.location.pathname + window.location.search,
      documentX: document.documentElement.scrollWidth - window.innerWidth,
      mainX: main ? main.scrollWidth - main.clientWidth : 0,
      scrollerX: Math.max(0, ...scrollerWidths),
      headers: header,
      h1,
      smallTargets,
      hoverOnlyControls: hoverOnlyControls.map((element) => element.getAttribute('aria-label') || element.textContent?.trim().slice(0, 48) || element.tagName),
      messageActions: [...document.querySelectorAll<HTMLElement>('main button[aria-label^="Message actions for"]')]
        .map((element) => ({ opacity: getComputedStyle(element).opacity, visibility: getComputedStyle(element).visibility, height: Math.round(element.getBoundingClientRect().height) })),
    };
  });
}

test('every routed page fits phone and desktop layouts', { session: 'admin' }, async ({ app, browser, screen }) => {
  await app.open('/');
  const cookie = await adminCookie(app.baseUrl);
  const projects = await api(app, '/api/projects', undefined, cookie) as Array<{ id: string; path: string }>;
  const project = projects.find((entry) => entry.path === REPO)
    ?? await api(app, '/api/projects', { path: REPO }, cookie);
  const session = await api(app, '/api/sessions', {
    harness: 'fake',
    model: 'fake-model',
    projectId: project.id,
  }, cookie);
  const sent = await api(app, `/api/sessions/${session.id}/prompt`, { text: 'Layout sweep message fixture' }, cookie);
  await waitForTurn(app, session.id, sent.seq, cookie);

  // Load the fixtures into the browser's signed-in App state before traversing
  // its route table. History/popstate keeps the authenticated page mounted.
  await app.open('/');
  const cases = routes(project.id, session.id, sent.seq);
  for (const route of cases) {
    expect(parsePath(route.path).name).toBe(route.route);
  }

  try {
    for (const size of SIZES) {
      await browser.setViewport({ width: size.width, height: size.height });
      for (const route of cases) {
        await visit(browser, route.path);
        if (route.route === 'session' || route.route === 'project') {
          await expect(screen.getByRole('region', 'Transcript')).toBeVisible();
        } else if (route.route !== 'ticket' && route.route !== 'project-ticket') {
          const heading = size.width < 768 ? route.heading : (route.desktopHeading ?? route.heading);
          await expect(screen.getByRole('heading', { name: heading!, level: 1 })).toBeVisible();
        }
        // No deterministic Run fixture exists in the e2e daemon; Ticket URLs
        // are still visited and their routed shell is measured at both widths.
        if (['run', 'project-run'].includes(route.route)) {
          await expect(screen.getByRole('alert')).toBeVisible();
        }

        const state = await measure(browser);
        const expectedPath = size.width >= 768 && route.desktopPath ? route.desktopPath : route.path;
        expect(state.path, `${route.path} at ${size.width}px`).toBe(expectedPath);
        expect(state.documentX, `${route.path} document at ${size.width}px`).toBeLessThanOrEqual(0);
        expect(state.mainX, `${route.path} main at ${size.width}px`).toBeLessThanOrEqual(0);
        expect(state.scrollerX, `${route.path} page scroller at ${size.width}px`).toBeLessThanOrEqual(0);
        expect(state.headers, `${route.path} page headers at ${size.width}px`).toBe(1);
        expect(state.h1, `${route.path} h1 count at ${size.width}px`).toBe(1);

        if (size.width < 768) {
          expect(state.smallTargets, `${route.path} targets below 44px`).toEqual([]);
          expect(state.hoverOnlyControls, `${route.path} hover-only controls`).toEqual([]);
          if (route.route === 'session') {
            expect(state.messageActions).not.toEqual([]);
            expect(state.messageActions.every((action: { opacity: string; visibility: string; height: number }) => action.opacity !== '0' && action.visibility !== 'hidden' && action.height >= 44)).toBe(true);
          }
        }
      }
    }
  } finally {
    await browser.setViewport({ width: 1280, height: 800 });
  }
});
