<script>
  import { ArrowRight, ShieldCheck } from '@lucide/svelte';
  import PageHeader from './ui/page-header.svelte';
  import { SETTINGS_GROUPS, settingsPath } from './settings.js';
  import { settingsIcons } from './settingsIcons.js';

  let { onNavigate = () => {} } = $props();

  function visit(event, path) {
    event.preventDefault();
    onNavigate(path);
  }
</script>

<div class="min-h-0 flex-1 overflow-auto px-4 pt-6 pb-12" data-testid="settings-hub">
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-8">
    <div class="flex flex-col gap-2">
      <PageHeader title="Settings" icon={ShieldCheck} scope="Global" class="animate-rise" />
      <p class="text-caption text-fg-3">Workspace defaults and your personal preferences.</p>
    </div>

    {#each SETTINGS_GROUPS as group (group.title)}
      <section class="animate-rise flex flex-col gap-2">
        <h2 class="px-1 text-sm font-semibold">{group.title}</h2>
        <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
          {#each group.items as item (item.id)}
            {@const Icon = settingsIcons[item.icon]}
            <a
              href={settingsPath(item.id)}
              class="flex min-h-16 items-center gap-3 p-4 transition-colors hover:bg-muted"
              onclick={(event) => visit(event, settingsPath(item.id))}
            >
              <span class="grid size-9 shrink-0 place-items-center rounded-xl bg-muted"><Icon class="size-4" /></span>
              <span class="min-w-0 flex-1">
                <span class="block text-sm font-medium">{item.label}</span>
                <span class="block truncate text-xs text-muted-foreground">{item.hint}</span>
              </span>
              <ArrowRight class="size-4 shrink-0 text-muted-foreground" />
            </a>
          {/each}
        </div>
      </section>
    {/each}
  </div>
</div>
