# 03: El macros work in the row for every harness

**What to build:** Macros are defined in settings: a name, a kind (prompt template or built-in action) and a template with `{{args}}`. Typing a registered macro in the row expands and delivers it, or runs the action. `/compress` ships as the first built-in and calls the existing compress request. Text that starts with a slash but names no macro is sent as a normal message. A macro that shadows a built-in is rejected or warned about at settings load, and el's macro wins on a collision.

**Blocked by:** 02-el-input-row, context-compress 06-summarizer-and-manual-compress

**Status:** done

- [x] A prompt template macro with arguments expands and delivers as a message
- [x] `/compress` in the row triggers compress for claude and pig with no plugin or key
- [x] Unknown slash text is sent as plain text
- [x] A duplicate macro name or a built-in collision is reported at settings load
- [x] Macro text goes through the same queue and hold rules as typed text
- [x] Tests for expansion, unknown text, validation and the compress trigger through the exec seam
- [x] Spec stories 8, 9, 10, 11, 12

Spec: `.scratch/archive/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
