// Shell context: App provides the phone drawer opener and its accessible name
// so every page header (ui/page-header.svelte) can render the menu button
// without each page knowing about the drawer.
export const SHELL_KEY = Symbol('el-shell');
