// The icon map: each domain concept has exactly one icon, named here once.
// Import from this file; never pick a lucide icon for a concept by hand.
import {
  Ban,
  Bot,
  CircleAlert,
  CircleCheck,
  CircleGauge,
  CirclePause,
  CircleX,
  Copy,
  Cpu,
  FileText,
  FolderGit2,
  Gauge,
  Gem,
  GitBranch,
  Layers,
  ListEnd,
  Loader,
  MessageCircle,
  MessageSquare,
  Brain,
  Pencil,
  Plug,
  RefreshCw,
  Route,
  RotateCcw,
  Share2,
  Shrink,
  Signpost,
  Sparkles,
  Terminal,
  Ticket,
  X,
} from '@lucide/svelte';

// Domain concepts. Settings, rail, buttons and rows all read from here.
export const icons = Object.freeze({
  session: MessageSquare,
  run: Layers,
  ticket: Ticket,
  subagent: Bot,
  steer: Signpost,
  followUp: ListEnd,
  report: FileText,
  memory: Brain,
  quota: Gauge,
  provider: Plug,
  project: FolderGit2,
  git: GitBranch,
  compress: Shrink,
  copy: Copy,
  share: Share2,
  edit: Pencil,
  retry: RotateCcw,
  dismiss: X,
  // Cancel a queued message: the row's own state glyph, not Dismiss.
  cancel: Ban,
  // Re-reading live data (quota, status). Retry is re-running a failed action.
  refresh: RefreshCw,
  // The model a Session or action runs on.
  model: Cpu,
});

// Status glyphs pair with a status word and the --status-* colour.
export const statusIcons = Object.freeze({
  working: Loader,
  'needs-you': CircleAlert,
  done: CircleCheck,
  failed: CircleX,
  paused: CirclePause,
});

export const statusLabels = Object.freeze({
  working: 'Working',
  'needs-you': 'Needs you',
  done: 'Done',
  failed: 'Failed',
  paused: 'Paused',
});

// Brand glyphs identify individual providers inside a Provider list; the
// Provider concept itself stays icons.provider.
const providerGlyphs = {
  claude: Sparkles,
  chatgpt: MessageCircle,
  'openai-codex': MessageCircle,
  'opencode-go': Terminal,
  openrouter: Route,
  'agy-gemini': Gem,
  'agy-3p': Gem,
};

export function providerGlyph(key) {
  return providerGlyphs[key] ?? CircleGauge;
}
