import { test } from '@e2e-dev/web';
import { expect } from 'e2e';

const REPO = '/tmp/el-e2e/repo';

async function api(app: { baseUrl: string | undefined }, path: string, body?: unknown) {
  const res = await fetch(`${app.baseUrl}${path}`, {
    method: body ? 'POST' : 'GET',
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  return res.json();
}

// Start every test on the fake harness so no real agent runs.
async function openWithFake({ app, browser }: any) {
  await app.open('/');
  await browser.evaluate(() => localStorage.setItem('eb.pref', JSON.stringify({ harness: 'fake', model: 'fake-model' })));
  await app.open('/');
}

test('add a project from the sidebar and keep it after reload', async ({ app, screen, browser }) => {
  await openWithFake({ app, browser });
  await screen.getByRole('button', 'Add project').click();
  await screen.getByLabel('Folder').fill('/tmp/el-e2e/add-me');
  await screen.getByLabel('Folder').press('Enter');
  await expect(screen.getByRole('region', 'add-me')).toBeVisible();
  // Adding drops you into a draft chat in that project.
  await expect(screen.getByText('What should we do in add-me?')).toBeVisible();

  await app.open('/');
  await expect(screen.getByRole('region', 'add-me')).toBeVisible();
});

test('a chat started in a project lists under it and uses its folder', async ({ app, screen, browser }) => {
  const project = await api(app, '/api/projects', { path: REPO });
  await openWithFake({ app, browser });

  await screen.getByRole('button', `New chat in ${project.name}`).click();
  await expect(screen.getByText(`What should we do in ${project.name}?`)).toBeVisible();
  await screen.getByLabel('message').fill('hello from e2e');
  await screen.getByRole('button', 'Send').click();

  await expect(screen.getByText('hello from e2e')).toBeVisible();
  const sessions = await api(app, '/api/sessions');
  const mine = sessions.find((s: any) => s.projectId === project.id);
  expect(mine.workDir).toBe(REPO);
  await expect(screen.getByRole('region', project.name).getByRole('navigation', 'Sessions')).toBeVisible();
});

test('the git panel shows the branch, changes and a diff', async ({ app, screen, browser }) => {
  const project = await api(app, '/api/projects', { path: REPO });
  await openWithFake({ app, browser });
  await screen.getByRole('button', `New chat in ${project.name}`).click();

  await screen.getByRole('button', /^main/).click();
  await expect(screen.getByText('Changes (2)')).toBeVisible();
  await expect(screen.getByText('first commit')).toBeVisible();
  await screen.getByText('a.txt').click();
  await expect(screen.getByText('+two')).toBeVisible();
  await screen.getByText('b.txt').click();
  await expect(screen.getByText('+new')).toBeVisible();
});

test('the top New chat button starts a draft in a project', async ({ app, screen, browser }) => {
  const project = await api(app, '/api/projects', { path: REPO });
  await openWithFake({ app, browser });
  await screen.getByRole('button', 'New chat', { exact: true }).click();
  await expect(screen.getByText(/What should we do in/)).toBeVisible();
  await expect(screen.getByRole('region', project.name)).toBeVisible();
});
