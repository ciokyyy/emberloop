# 05: Settings tab

**What to build:** A Settings tab lists every role slot with harness, model and effort, plus decision model and threshold, each with its source (file or default). Choosing a harness offers models from that harness's cached `Models()` list (free text allowed); effort shows as a row of the levels the model supports with the chosen one highlighted (`low  medium [high]  xhigh  max`), hidden when there are none. Switching harness keeps model and effort when valid, otherwise resets them with a notice. `r` refreshes model lists. Archive size is shown. Saves go through the global settings load/save functions.

**Blocked by:** 01, global-settings 02

**Status:** done

- [x] Edits persist via the shared settings functions; the CLI sees them
- [x] No model lists hard-coded in the TUI
- [x] Effort row reflects the selected model's levels
- [x] Model tests: change harness, model and effort via keys and clicks, assert the saved file
