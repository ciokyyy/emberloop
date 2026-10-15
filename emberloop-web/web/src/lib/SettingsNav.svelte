<script>
  import { SETTINGS_GROUPS, settingsPath } from './settings.js';
  import { settingsIcons } from './settingsIcons.js';

  let { current = null, onNavigate = () => {} } = $props();

  function visit(event, path) {
    event.preventDefault();
    onNavigate(path);
  }
</script>

<nav class="flex min-h-0 flex-1 flex-col gap-4 overflow-auto" aria-label="Settings navigation" data-testid="settings-nav">
  {#each SETTINGS_GROUPS as group (group.title)}
    <div class="flex flex-col gap-0.5">
      <h2 class="px-2 pb-1 text-xs font-medium text-muted-foreground">{group.title}</h2>
      {#each group.items as item (item.id)}
        {@const Icon = settingsIcons[item.icon]}
        <a
          href={settingsPath(item.id)}
          aria-current={current === item.id ? 'page' : undefined}
          class="flex min-h-11 items-center gap-2.5 rounded-xl px-2.5 text-sm transition-colors {current === item.id ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
          onclick={(event) => visit(event, settingsPath(item.id))}
        >
          <Icon class="size-4 shrink-0" />
          <span class="min-w-0 flex-1 truncate">{item.label}</span>
        </a>
      {/each}
    </div>
  {/each}
</nav>
