<!-- Settings hub: every concern as a row, grouped Workspace and You. -->
<script>
  import { ChevronRight, ShieldCheck } from '@lucide/svelte';
  import Chip from './ui/chip.svelte';
  import PageHeader from './ui/page-header.svelte';
  import Panel from './ui/panel.svelte';
  import Row from './ui/row.svelte';
  import Section from './ui/section.svelte';
  import { SETTINGS_GROUPS, settingsPath } from './settings.js';
  import { settingsIcons } from './settingsIcons.js';

  let { onNavigate = () => {} } = $props();

  function visit(event, path) {
    event.preventDefault();
    onNavigate(path);
  }
</script>

<div class="min-h-0 flex-1 overflow-auto px-4 pt-6 pb-[max(3rem,env(safe-area-inset-bottom))] md:px-6" data-testid="settings-hub">
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
    <PageHeader
      title="Settings"
      icon={ShieldCheck}
      scope="Global"
      hint="Workspace defaults and your personal preferences."
      class="animate-rise"
    />

    {#each SETTINGS_GROUPS as group (group.title)}
      <Section title={group.title} class="animate-rise">
        <Panel divided={false} class="gap-0.5 p-1">
          {#each group.items as item (item.id)}
            <Row
              href={settingsPath(item.id)}
              icon={settingsIcons[item.icon]}
              title={item.label}
              class="min-h-16"
              meta={item.hint}
              onclick={(event) => visit(event, settingsPath(item.id))}
            >
              {#snippet badges()}{#if item.scope === 'Admin'}<Chip tone="outline">Admin</Chip>{/if}{/snippet}
              {#snippet trailing()}<ChevronRight class="size-4 text-fg-3" aria-hidden="true" />{/snippet}
            </Row>
          {/each}
        </Panel>
      </Section>
    {/each}
  </div>
</div>
