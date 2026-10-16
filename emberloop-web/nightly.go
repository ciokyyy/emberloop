package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/emberloop-web/memory"
)

// Nightly job: snapshot, catch up, reembed, fold and consolidate Memories,
// promote candidates, then run the legacy profile and anchor stages. The
// scheduler checks each minute after the local 03:00 due boundary and starts
// at the next idle opportunity; the Memory page can also run it on demand.
// Every stage writes a ledger row per User bank; a snapshot failure aborts
// before any mine write, while later stage failures leave later stages running.
// Nothing is ever deleted: edits are new versions and status changes, and
// tests assert row counts before and after.

const (
	// nightlyDistillCap caps one distill input at 15 KB of redacted text.
	nightlyDistillCap = 15 * 1024
	// nightlyFlushOK is the distill sentinel for no durable content.
	nightlyFlushOK = "FLUSH_OK"
	// nightlyDistillMaxTokens caps one distill or profile reply; briefs run short.
	nightlyDistillMaxTokens = 1024
	// nightlyDistillTimeout bounds one distill or profile call.
	nightlyDistillTimeout = 2 * time.Minute
)

// Nightly stage names, one ledger row per stage per run per bank.
const (
	nightlyStageSnapshot          = "nightly_snapshot"
	nightlyStageMine              = "nightly_mine"
	nightlyStageDistill           = "nightly_distill"
	nightlyStageReembed           = "nightly_reembed"
	nightlyStageFold              = "nightly_fold"
	nightlyStagePromote           = "nightly_promote"
	nightlyStageConsolidate       = "nightly_consolidate"
	nightlyStagePair              = "nightly_pair"
	nightlyStageMemoryConsolidate = "nightly_memory_consolidate"
	nightlyStageProfile           = "nightly_profile"
	nightlyStageAnchors           = "nightly_anchors"
	nightlyStageSummary           = "nightly_summary"
)

// NightlyStageResult is one stage of one run.
type NightlyStageResult struct {
	Stage  string         `json:"stage"`
	Status string         `json:"status"`
	Counts map[string]any `json:"counts"`
	Detail string         `json:"detail"`
}

// NightlyResult is one job run: its stages in order.
type NightlyResult struct {
	RunID     string               `json:"runId"`
	StartedAt time.Time            `json:"startedAt"`
	Aborted   bool                 `json:"aborted"`
	Error     string               `json:"error,omitempty"`
	Stages    []NightlyStageResult `json:"stages"`
}

// nightlyOwners lists every User ID with a bank: signed-up Users, Session
// owners (tests seed banks without signup) and existing bank files. Sorted,
// deduped, never empty-checked: callers treat no owners as NO_DATA.
func (d *Daemon) nightlyOwners() []string {
	seen := make(map[string]bool)
	if f, err := d.users.read(); err == nil {
		for _, u := range f.Users {
			if strings.TrimSpace(u.ID) != "" {
				seen[u.ID] = true
			}
		}
	}
	d.mu.Lock()
	for _, s := range d.sessions {
		if strings.TrimSpace(s.meta.Owner) != "" {
			seen[s.meta.Owner] = true
		}
	}
	d.mu.Unlock()
	if entries, err := os.ReadDir(filepath.Join(d.memoryRoot(), "users")); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
				continue
			}
			if id := strings.TrimSuffix(e.Name(), ".db"); strings.TrimSpace(id) != "" {
				seen[id] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// capDistillInput cuts s to at most nightlyDistillCap bytes, trimming only a
// trailing partial rune so CJK text stays valid UTF-8. Earlier bytes pass
// through untouched, even when they are not valid UTF-8. The caller redacts
// first, so the cap applies to redacted text leaving the Daemon.
func capDistillInput(s string) string {
	if len(s) <= nightlyDistillCap {
		return s
	}
	cut := s[:nightlyDistillCap]
	for len(cut) > 0 {
		if r, size := utf8.DecodeLastRuneInString(cut); r != utf8.RuneError || size > 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut
}

// distillSystemPrompt asks for a brief or the FLUSH_OK sentinel.
func distillSystemPrompt() string {
	return "You read redacted user corrections from one Session. " +
		"Return a short brief (at most 500 characters) of durable preferences, corrections or decisions " +
		"meant for future work, not only this task. When nothing durable exists, reply with exactly " + nightlyFlushOK + ". " +
		"Reply with only the brief or " + nightlyFlushOK + ", no other text."
}

// profileSystemPrompt asks for JSON ops or none.
func profileSystemPrompt() string {
	return "You propose Profile patch ops from durable notes. " +
		"Reply with JSON only: {\"ops\":[{\"op\":\"add\",\"text\":\"...\",\"tag\":\"workflow\"|\"style\"|\"other\"}," +
		"{\"op\":\"edit\",\"id\":\"l1\",\"text\":\"...\"},{\"op\":\"retire\",\"id\":\"l2\"}]}. " +
		"When no stable signal exists, reply with exactly none. No other text."
}

// nightlyLLMCall runs one cheap distill or profile-proposal completion: the
// same bare pig one-shot the compress summarizer uses, with no tools and no
// Session behind it. The user text is redacted for distill before it is sent.
// seam scripts the call in tests (distillOneShot, profileOneShot); nil runs
// the bare pig one-shot.
func (d *Daemon) nightlyLLMCall(seam func(model, system, user string) (string, error), model, system, user string) (string, error) {
	redacted := memory.ForDistill(user)
	if seam != nil {
		return seam(model, system, redacted)
	}
	return pigOneShot("", model, system, redacted, nightlyDistillMaxTokens, nightlyDistillTimeout)
}

// nightlyLedger writes one row per owner bank and reports how many banks it
// could not open. A bank that cannot open gets no row here; stages count that
// as failed so a silent skip never hides. A ledger write failure itself never
// fails the stage it follows: the stage status already names the outcome, and
// the job must leave later stages running.
func (d *Daemon) nightlyLedger(ctx context.Context, runID, stage, status string, counts map[string]any, detail string, owners []string) (unopenable int) {
	if counts == nil {
		counts = map[string]any{}
	}
	raw, _ := json.Marshal(counts)
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			unopenable++
			continue
		}
		_, _ = bank.RecordLedger(ctx, memory.LedgerRow{
			RunID: runID, Stage: stage, Status: status,
			CountsJSON: string(raw), Detail: detail,
		})
	}
	return unopenable
}

// nightlyRecordLedger writes one row to an open bank. Failures stay silent:
// the stage status already names the outcome.
func nightlyRecordLedger(ctx context.Context, bank memory.Memory, runID, stage, status string, counts map[string]any, detail string) {
	if counts == nil {
		counts = map[string]any{}
	}
	raw, _ := json.Marshal(counts)
	_, _ = bank.RecordLedger(ctx, memory.LedgerRow{
		RunID: runID, Stage: stage, Status: status,
		CountsJSON: string(raw), Detail: detail,
	})
}

// nightlyOpenBanks opens every owner's bank for a stage. Banks that cannot
// open are counted, never skipped silently: without a bank there is no
// snapshot, no pairing and no ledger row, so the stage must show it.
func (d *Daemon) nightlyOpenBanks(owners []string) (map[string]memory.Memory, int, string) {
	open := make(map[string]memory.Memory, len(owners))
	failed := 0
	var firstErr string
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = "bank for " + owner + ": " + err.Error()
			}
			continue
		}
		open[owner] = bank
	}
	return open, failed, firstErr
}

// nightlyInjectedFailure is the deterministic test seam for the abort path
// (snapshot) and the continue paths (every later stage): when the test sets
// nightlyFailStage, the stage records its FAILED ledger row and stops. Real
// failure paths keep their own tests: a blocked snapshot directory, a
// failing distill call, a non-JSON profile reply and a refused proposal.
func (d *Daemon) nightlyInjectedFailure(ctx context.Context, runID, stage string, owners []string) *NightlyStageResult {
	if d.nightlyFailStage != stage {
		return nil
	}
	detail := "nightly: injected " + stage + " failure"
	d.nightlyLedger(ctx, runID, stage, memory.LedgerFailed, map[string]any{"owners": len(owners)}, detail, owners)
	return &NightlyStageResult{Stage: stage, Status: memory.LedgerFailed,
		Counts: map[string]any{"owners": len(owners)}, Detail: detail}
}

// nightlySnapshot copies every bank with VACUUM INTO. Any bank failing aborts
// the run before any mine write.
func (d *Daemon) nightlySnapshot(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageSnapshot, owners); fail != nil {
		return *fail
	}
	if len(owners) == 0 {
		return NightlyStageResult{Stage: nightlyStageSnapshot, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": 0}, Detail: "no banks to snapshot"}
	}
	banks, failed, firstErr := d.nightlyOpenBanks(owners)
	taken, kept, pruned := 0, 0, 0
	for _, owner := range owners {
		bank, ok := banks[owner]
		if !ok {
			continue
		}
		res, err := bank.Snapshot(ctx)
		status := memory.LedgerSuccess
		detail := "snapshot taken"
		counts := map[string]any{"taken": res.Taken, "kept": res.Kept, "pruned": res.Pruned}
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			status = memory.LedgerFailed
			detail = "snapshot failed: " + err.Error()
			counts = map[string]any{"owners": len(owners)}
		} else if res.Taken {
			taken++
			kept += res.Kept
			pruned += res.Pruned
			if res.Path != "" {
				detail = "snapshot taken: " + filepath.Base(res.Path)
			}
		} else {
			status = memory.LedgerNoData
			detail = "snapshot skipped: newest under 6 hours"
		}
		nightlyRecordLedger(ctx, bank, runID, nightlyStageSnapshot, status, counts, detail)
	}
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("snapshotted %d banks: %d taken, %d failed", len(owners), taken, failed)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("snapshot failed for %d of %d banks: %s", failed, len(owners), firstErr)
	} else if taken == 0 {
		status = memory.LedgerNoData
		detail = fmt.Sprintf("no snapshot needed for %d banks (newest under 6 hours)", len(owners))
	}
	return NightlyStageResult{Stage: nightlyStageSnapshot, Status: status,
		Counts: map[string]any{"owners": len(owners), "taken": taken, "kept": kept, "pruned": pruned, "failed": failed},
		Detail: detail}
}

// nightlyMine mines every settled Session once. One Session failing never
// stops the others; the errors join and the stage is FAILED but later stages
// still run.
func (d *Daemon) nightlyMine(ctx context.Context, runID string, owners []string) ([]MineResult, NightlyStageResult) {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageMine, owners); fail != nil {
		return nil, *fail
	}
	catchupSessions, catchupErr := d.nightlyCatchupIngest(ctx)
	results, err := d.MineSettledSessions()
	scanned, kept, stored, dups, dropped, promoted := 0, 0, 0, 0, 0, 0
	for _, r := range results {
		scanned += r.Scanned
		kept += r.Kept
		stored += r.Stored
		dups += r.Duplicates
		dropped += r.Dropped
		promoted += r.Promoted
	}
	failed := 0
	var firstErr string
	if catchupErr != nil {
		failed++
		firstErr = catchupErr.Error()
	}
	if err != nil {
		failed++
		firstErr = err.Error()
	}
	if _, openFailed, openErr := d.nightlyOpenBanks(owners); openFailed > 0 {
		failed += openFailed
		if firstErr == "" {
			firstErr = openErr
		}
	}
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("caught up %d Sessions and mined %d legacy Sessions: %d scanned, %d kept, %d stored, %d duplicates, %d promoted", catchupSessions, len(results), scanned, kept, stored, dups, promoted)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("mine partial: %s; %s", firstErr, detail)
	} else if stored == 0 && promoted == 0 && catchupSessions == 0 {
		status = memory.LedgerNoData
		detail = "no settled Sessions to catch up or mine"
	}
	counts := map[string]any{"catchup_sessions": catchupSessions, "sessions": len(results), "scanned": scanned, "kept": kept, "stored": stored, "duplicates": dups, "dropped": dropped, "promoted": promoted, "failed": failed}
	d.nightlyLedger(ctx, runID, nightlyStageMine, status, counts, detail, owners)
	return results, NightlyStageResult{Stage: nightlyStageMine, Status: status, Counts: counts, Detail: detail}
}

// nightlySessionUserTexts returns the miner-readable user texts of one Session
// for the distill input: sender=user prompts, steers and follow-ups only, so
// Handoffs, injections and Daemon-made prompts never reach the distill LLM.
func (d *Daemon) nightlySessionUserTexts(id string) string {
	d.mu.Lock()
	s, ok := d.sessions[id]
	var evs []Event
	if ok {
		evs = append([]Event(nil), s.events...)
	}
	d.mu.Unlock()
	var parts []string
	for _, e := range evs {
		if !MinerReadable(e) {
			continue
		}
		if t := strings.TrimSpace(e.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// nightlyDistill calls one cheap LLM per Session with new observations. A
// FLUSH_OK reply means nothing durable and enqueues nothing; a brief is
// recorded for the profile stage. The input is redacted and capped at 15 KB.
func (d *Daemon) nightlyDistill(ctx context.Context, runID string, owners []string, mined []MineResult) (map[string]string, map[string]string, NightlyStageResult) {
	briefs := make(map[string]string)
	sessionOwner := make(map[string]string)
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageDistill, owners); fail != nil {
		return briefs, sessionOwner, *fail
	}
	var targets []string
	for _, r := range mined {
		if r.Stored > 0 {
			targets = append(targets, r.SessionID)
		}
	}
	sort.Strings(targets)
	if len(targets) == 0 {
		d.nightlyLedger(ctx, runID, nightlyStageDistill, memory.LedgerNoData,
			map[string]any{"sessions": 0}, "no sessions with new observations to distill", owners)
		return briefs, sessionOwner, NightlyStageResult{Stage: nightlyStageDistill, Status: memory.LedgerNoData,
			Counts: map[string]any{"sessions": 0}, Detail: "no sessions with new observations to distill"}
	}
	set := d.deciderSettingsOf()
	model := strings.TrimSpace(set.Model)
	needModel := model == "" && d.distillOneShot == nil
	if needModel {
		d.nightlyLedger(ctx, runID, nightlyStageDistill, memory.LedgerNoData,
			map[string]any{"sessions": len(targets)}, "no memory model: distill skipped, candidates stay unconfirmed", owners)
		return briefs, sessionOwner, NightlyStageResult{Stage: nightlyStageDistill, Status: memory.LedgerNoData,
			Counts: map[string]any{"sessions": len(targets)}, Detail: "no memory model: distill skipped"}
	}
	distilled, flushOK, failed := 0, 0, 0
	var firstErr string
	for _, id := range targets {
		d.mu.Lock()
		owner := ""
		if s, ok := d.sessions[id]; ok {
			owner = s.meta.Owner
		}
		d.mu.Unlock()
		if owner != "" {
			sessionOwner[id] = owner
		}
		raw := d.nightlySessionUserTexts(id)
		if strings.TrimSpace(raw) == "" {
			// No miner-readable text but new observations: the log moved
			// under the miner. Only the cheap LLM may return FLUSH_OK, so
			// this counts as a failure, never as a sentinel.
			failed++
			if firstErr == "" {
				firstErr = "no miner-readable text for session " + id
			}
			continue
		}
		input := capDistillInput(memory.ForDistill(raw))
		reply, err := d.nightlyLLMCall(d.distillOneShot, model, distillSystemPrompt(), input)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		distilled++
		if strings.TrimSpace(reply) == nightlyFlushOK {
			flushOK++
			continue
		}
		if strings.TrimSpace(reply) == "" {
			failed++
			if firstErr == "" {
				firstErr = "empty distill reply for session " + id
			}
			continue
		}
		briefs[id] = strings.TrimSpace(reply)
	}
	briefN := len(briefs)
	if _, openFailed, openErr := d.nightlyOpenBanks(owners); openFailed > 0 {
		failed += openFailed
		if firstErr == "" {
			firstErr = openErr
		}
	}
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("distilled %d sessions: %d briefs, %d %s", len(targets), briefN, flushOK, nightlyFlushOK)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("distill partial: %d failed (%s); %s", failed, firstErr, detail)
	} else if briefN == 0 {
		status = memory.LedgerNoData
	}
	counts := map[string]any{"sessions": len(targets), "distilled": distilled, "briefs": briefN, "flush_ok": flushOK, "failed": failed}
	d.nightlyLedger(ctx, runID, nightlyStageDistill, status, counts, detail, owners)
	return briefs, sessionOwner, NightlyStageResult{Stage: nightlyStageDistill, Status: status, Counts: counts, Detail: detail}
}

// nightlyPair runs contradiction pairing over new candidates: every pending
// rule proposal that arrived without a relation. For each, FTS5 finds stored
// Rules with the same subject key or high overlap, and the relation
// (duplicate, refines or contradicts) is written back onto the queue payload
// with the related Rule, so the approver sees the conflict. Proposals that
// already carry a relation (paired at promotion time or by an earlier run)
// are left untouched. The stage writes queue annotations plus ledger rows: it
// never deletes.
func (d *Daemon) nightlyPair(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStagePair, owners); fail != nil {
		return *fail
	}
	if len(owners) == 0 {
		return NightlyStageResult{Stage: nightlyStagePair, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": 0}, Detail: "no banks to pair"}
	}
	pending, already, annotated, unpaired, failed := 0, 0, 0, 0, 0
	var firstErr string
	banks, openFailed, openErr := d.nightlyOpenBanks(owners)
	failed += openFailed
	if firstErr == "" {
		firstErr = openErr
	}
	for _, owner := range owners {
		bank, ok := banks[owner]
		if !ok {
			continue
		}
		items, err := bank.Queue(ctx)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStagePair, memory.LedgerFailed,
				map[string]any{"annotated": 0}, "pair failed: "+err.Error())
			continue
		}
		var mine []memory.QueueItem
		for _, it := range items {
			if it.Kind == memory.QueueRule && it.Status == memory.QueuePending {
				mine = append(mine, it)
			}
		}
		hit, wrote, single := 0, 0, 0
		for _, it := range mine {
			proposal, err := memory.DecodeRememberProposal(it.Payload)
			if err != nil || strings.TrimSpace(proposal.Text) == "" {
				continue
			}
			if strings.TrimSpace(proposal.Relation) != "" {
				hit++
				continue
			}
			sameKey, err := bank.RulesBySubjectKey(ctx, proposal.SubjectKey)
			if err != nil {
				failed++
				if firstErr == "" {
					firstErr = err.Error()
				}
				break
			}
			recalled, err := bank.Recall(ctx, proposal.Text, memory.MaxRecall)
			if err != nil {
				failed++
				if firstErr == "" {
					firstErr = err.Error()
				}
				break
			}
			seen := make(map[string]memory.Rule)
			for _, r := range sameKey {
				seen[r.RuleID] = r
			}
			for _, r := range recalled {
				seen[r.RuleID] = r
			}
			var all []memory.Rule
			for _, r := range seen {
				all = append(all, r)
			}
			sort.Slice(all, func(i, j int) bool { return all[i].RuleID < all[j].RuleID })
			related, rel := memory.PairCandidate(proposal.Text, proposal.SubjectKey, all)
			if related == "" {
				single++
				continue
			}
			if err := bank.AnnotateQueueRelation(ctx, it.ID, rel, related); err != nil {
				failed++
				if firstErr == "" {
					firstErr = err.Error()
				}
				continue
			}
			hit++
			wrote++
		}
		pending += len(mine)
		already += hit - wrote
		annotated += wrote
		unpaired += single
		status := memory.LedgerSuccess
		detail := fmt.Sprintf("paired %d of %d pending rule proposals (%d newly annotated)", hit, len(mine), wrote)
		counts := map[string]any{"pending": len(mine), "paired": hit, "annotated": wrote, "unpaired": single}
		if len(mine) == 0 {
			status = memory.LedgerNoData
			detail = "no pending rule proposals to pair"
		}
		raw, _ := json.Marshal(counts)
		_, _ = bank.RecordLedger(ctx, memory.LedgerRow{RunID: runID, Stage: nightlyStagePair,
			Status: status, CountsJSON: string(raw), Detail: detail})
	}
	paired := already + annotated
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("paired %d of %d pending rule proposals (%d newly annotated)", paired, pending, annotated)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("pair partial: %s; %s", firstErr, detail)
	} else if pending == 0 {
		status = memory.LedgerNoData
		detail = "no pending rule proposals to pair"
	}
	return NightlyStageResult{Stage: nightlyStagePair, Status: status,
		Counts: map[string]any{"owners": len(owners), "pending": pending, "paired": paired, "annotated": annotated, "unpaired": unpaired, "failed": failed},
		Detail: detail}
}

// nightlyProfile proposes Profile patches from distilled briefs: add, edit or
// retire ops by line ID, JSON-only, none when no stable signal. Proposals go
// to the queue; a human approval writes the version.
func (d *Daemon) nightlyProfile(ctx context.Context, runID string, owners []string, briefs map[string]string, sessionOwner map[string]string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageProfile, owners); fail != nil {
		return *fail
	}
	if len(owners) == 0 {
		return NightlyStageResult{Stage: nightlyStageProfile, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": 0}, Detail: "no banks for profile proposals"}
	}
	byOwner := make(map[string][]string)
	for id, brief := range briefs {
		owner := sessionOwner[id]
		if owner == "" {
			d.mu.Lock()
			if s, ok := d.sessions[id]; ok {
				owner = s.meta.Owner
			}
			d.mu.Unlock()
		}
		if owner == "" {
			continue
		}
		byOwner[owner] = append(byOwner[owner], brief)
	}
	set := d.deciderSettingsOf()
	model := strings.TrimSpace(set.Model)
	needModel := model == "" && d.profileOneShot == nil
	proposed, none, failed := 0, 0, 0
	var firstErr string
	banks, openFailed, openErr := d.nightlyOpenBanks(owners)
	failed += openFailed
	if firstErr == "" {
		firstErr = openErr
	}
	for _, owner := range owners {
		bank, ok := banks[owner]
		if !ok {
			continue
		}
		d.remindOldProfilePatches(ctx, bank, owner)
		d.sendMemoryDigest(ctx, bank, owner)
		notes := byOwner[owner]
		if len(notes) == 0 || needModel {
			none++
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerNoData,
				map[string]any{"proposed": 0}, "no stable signal for a profile patch")
			continue
		}
		sort.Strings(notes)
		current, err := bank.GetProfile(ctx)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerFailed,
				map[string]any{"proposed": 0}, "profile failed: "+err.Error())
			continue
		}
		rendered, _ := memory.RenderProfile(current)
		var b strings.Builder
		b.WriteString("Current profile:\n")
		b.WriteString(rendered)
		b.WriteString("\n\nRecent durable notes:\n")
		b.WriteString(strings.Join(notes, "\n\n"))
		input := capDistillInput(memory.ForDistill(b.String()))
		reply, err := d.nightlyLLMCall(d.profileOneShot, model, profileSystemPrompt(), input)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerFailed,
				map[string]any{"proposed": 0}, "profile failed: "+err.Error())
			continue
		}
		trimmed := strings.TrimSpace(reply)
		if strings.EqualFold(trimmed, "none") || trimmed == "" {
			none++
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerNoData,
				map[string]any{"proposed": 0}, "profile proposer returned none")
			continue
		}
		var patch memory.ProfilePatch
		if err := json.Unmarshal([]byte(trimmed), &patch); err != nil {
			failed++
			if firstErr == "" {
				firstErr = "profile reply is not JSON: " + err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerFailed,
				map[string]any{"proposed": 0}, "profile proposer returned non-JSON")
			continue
		}
		if len(patch.Ops) == 0 {
			none++
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerNoData,
				map[string]any{"proposed": 0}, "profile proposer returned empty ops")
			continue
		}
		if _, err := d.proposeProfilePatch(ctx, owner, patch, "nightly", "", owner); err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerFailed,
				map[string]any{"proposed": 0}, "profile proposal refused: "+err.Error())
			continue
		}
		proposed++
		nightlyRecordLedger(ctx, bank, runID, nightlyStageProfile, memory.LedgerSuccess,
			map[string]any{"proposed": 1}, "profile patch proposed from nightly briefs")
	}
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("proposed %d profile patches (%d with no signal)", proposed, none)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("profile partial: %s; %s", firstErr, detail)
	} else if proposed == 0 {
		status = memory.LedgerNoData
	}
	return NightlyStageResult{Stage: nightlyStageProfile, Status: status,
		Counts: map[string]any{"owners": len(owners), "proposed": proposed, "none": none, "failed": failed},
		Detail: detail}
}

func (d *Daemon) remindOldProfilePatches(ctx context.Context, bank memory.Memory, owner string) {
	d.remindOldProfilePatchesAt(ctx, bank, owner, d.now())
}

func (d *Daemon) remindOldProfilePatchesAt(ctx context.Context, bank memory.Memory, owner string, now time.Time) {
	d.profileReminderMu.Lock()
	defer d.profileReminderMu.Unlock()
	items, err := bank.Queue(ctx)
	if err != nil {
		return
	}
	ledger, err := bank.Ledger(ctx)
	if err != nil {
		return
	}
	reminded := make(map[string]bool)
	for _, row := range ledger {
		if row.Stage == "profile_patch_reminder" {
			reminded[row.Detail] = true
		}
	}
	for _, item := range items {
		if item.Kind != memory.QueueProfilePatch || item.Status != memory.QueuePending || item.CreatedAt.IsZero() || now.Sub(item.CreatedAt) < 48*time.Hour {
			continue
		}
		key := itoa(item.ID)
		if reminded[key] {
			continue
		}
		// Persist the claim before attempting delivery. The lock serializes
		// concurrent nightly invocations; a failed ledger write sends nothing.
		if _, err := bank.RecordLedger(ctx, memory.LedgerRow{Stage: "profile_patch_reminder", Status: memory.LedgerSuccess, Detail: key}); err != nil {
			continue
		}
		reminded[key] = true
		d.pushProfilePatch(owner, "", item.ID, "Reminder: this Profile patch is still awaiting your review.")
	}
}

func (d *Daemon) sendMemoryDigest(ctx context.Context, bank memory.Memory, owner string) {
	if !d.push.digestEnabled(owner) {
		return
	}
	rows, err := bank.Ledger(ctx)
	if err != nil {
		return
	}
	today := d.now().In(time.Local).Format("2006-01-02")
	learned := 0
	for _, row := range rows {
		if row.CreatedAt.In(time.Local).Format("2006-01-02") != today {
			continue
		}
		if row.Stage == "memory_digest_sent" && row.Detail == today {
			return
		}
		if row.Stage != "ingest" && row.Stage != nightlyStageFold {
			continue
		}
		var counts map[string]int
		if json.Unmarshal([]byte(row.CountsJSON), &counts) == nil {
			learned += counts["stored"] + counts["memories"]
		}
	}
	payload := pushPayload(pushMessage{Title: "Memory daily digest", Body: fmt.Sprintf("learned %d things today, tap to review", learned), Tag: "memory-digest:" + today, Key: "memory-digest:" + today, Path: "/memory"})
	go d.push.notify(payload)
	_, _ = bank.RecordLedger(ctx, memory.LedgerRow{Stage: "memory_digest_sent", Status: memory.LedgerSuccess, Detail: today})
}

// nightlyMainRepoDir resolves dir to its main repo workdir through the git
// common dir: a linked worktree shares the main repo's object store, so the
// anchor check runs against the current main branch checkout instead of a
// feature worktree HEAD. Dirs that do not resolve pass through unchanged.
func nightlyMainRepoDir(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return dir
	}
	common := strings.TrimSpace(stdout.String())
	if common == "" {
		return dir
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	if main := filepath.Dir(common); main != dir {
		if st, err := os.Stat(main); err == nil && st.IsDir() {
			return main
		}
	}
	return dir
}

// nightlyAnchorDirs collects distinct Session working directories resolved to
// their main repo workdirs for the anchor check.
func (d *Daemon) nightlyAnchorDirs(ctx context.Context) []string {
	seen := make(map[string]bool)
	d.mu.Lock()
	for _, s := range d.sessions {
		if dir := strings.TrimSpace(s.meta.WorkDir); dir != "" {
			seen[dir] = true
		}
	}
	d.mu.Unlock()
	mains := make(map[string]bool)
	for dir := range seen {
		mains[nightlyMainRepoDir(ctx, dir)] = true
	}
	var out []string
	for dir := range mains {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

// nightlyAnchors refreshes dormant status against the current main branch: for
// each live Rule with anchors it runs git diff observed_commit..HEAD in the
// main repo checkout (Session workdirs resolve there through the git common
// dir) over the paths. Changed anchors inject with a flag, deleted anchors go
// dormant. Preferences carry no anchors and are never checked. The dormant
// count names Rules this run made dormant, not Rules already dormant.
func (d *Daemon) nightlyAnchors(ctx context.Context, runID string, owners []string) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageAnchors, owners); fail != nil {
		return *fail
	}
	if len(owners) == 0 {
		return NightlyStageResult{Stage: nightlyStageAnchors, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": 0}, Detail: "no banks for anchor check"}
	}
	workDirs := d.nightlyAnchorDirs(ctx)
	if len(workDirs) == 0 {
		d.nightlyLedger(ctx, runID, nightlyStageAnchors, memory.LedgerNoData,
			map[string]any{"owners": len(owners)}, "no working directories to check anchors against", owners)
		return NightlyStageResult{Stage: nightlyStageAnchors, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": len(owners), "workdirs": 0}, Detail: "no working directories to check anchors against"}
	}
	rules, dormant, failed := 0, 0, 0
	var firstErr string
	banks, openFailed, openErr := d.nightlyOpenBanks(owners)
	failed += openFailed
	if firstErr == "" {
		firstErr = openErr
	}
	for _, owner := range owners {
		bank, ok := banks[owner]
		if !ok {
			continue
		}
		live, err := bank.LiveRules(ctx)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageAnchors, memory.LedgerFailed,
				map[string]any{"rules": 0}, "anchors failed: "+err.Error())
			continue
		}
		var anchored []memory.Rule
		for _, r := range live {
			if len(r.Anchors) > 0 {
				anchored = append(anchored, r)
			}
		}
		if len(anchored) == 0 {
			nightlyRecordLedger(ctx, bank, runID, nightlyStageAnchors, memory.LedgerNoData,
				map[string]any{"rules": 0}, "no anchored rules to check")
			continue
		}
		wasDormant := nightlyDormantSet(ctx, bank, anchored)
		var verr error
		for _, dir := range workDirs {
			if _, verr = memory.ApplyVerification(ctx, bank, dir, anchored); verr != nil {
				break
			}
		}
		newDormant := 0
		for _, r := range anchored {
			if wasDormant[r.RuleID] {
				continue
			}
			if nightlyRuleDormant(ctx, bank, r.RuleID) {
				newDormant++
			}
		}
		rules += len(anchored)
		dormant += newDormant
		status := memory.LedgerSuccess
		detail := fmt.Sprintf("checked %d anchored rules against %d main checkouts: %d newly dormant", len(anchored), len(workDirs), newDormant)
		counts := map[string]any{"rules": len(anchored), "dormant": newDormant, "workdirs": len(workDirs)}
		if verr != nil {
			failed++
			if firstErr == "" {
				firstErr = verr.Error()
			}
			status = memory.LedgerFailed
			detail = fmt.Sprintf("anchors partial: %s; %s", verr.Error(), detail)
		}
		nightlyRecordLedger(ctx, bank, runID, nightlyStageAnchors, status, counts, detail)
	}
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("checked %d anchored rules: %d newly dormant", rules, dormant)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("anchors partial: %s; %s", firstErr, detail)
	} else if rules == 0 {
		status = memory.LedgerNoData
		detail = "no anchored rules to check"
	}
	return NightlyStageResult{Stage: nightlyStageAnchors, Status: status,
		Counts: map[string]any{"owners": len(owners), "rules": rules, "dormant": dormant, "failed": failed},
		Detail: detail}
}

// nightlyRuleDormant reports whether the latest version of ruleID is dormant.
// The anchor check uses it before and after verification so the ledger counts
// only Rules this run made dormant, never ones already dormant.
func nightlyRuleDormant(ctx context.Context, bank memory.Memory, ruleID string) bool {
	vers, err := bank.RuleVersions(ctx, ruleID)
	if err != nil || len(vers) == 0 {
		return false
	}
	return vers[len(vers)-1].Status == memory.StatusDormant
}

// nightlyDormantSet reports the anchored Rules already dormant before this
// run, so the ledger counts only transitions. LiveRules excludes dormant
// Rules, so the set is normally empty; it exists so a rerun never recounts.
func nightlyDormantSet(ctx context.Context, bank memory.Memory, anchored []memory.Rule) map[string]bool {
	out := make(map[string]bool)
	for _, r := range anchored {
		if nightlyRuleDormant(ctx, bank, r.RuleID) {
			out[r.RuleID] = true
		}
	}
	return out
}

// nightlySummary writes the run summary per bank: stage counts, queue size,
// approval latency and budget pressure, so the Memory page shows the last run
// without recomputing it.
func (d *Daemon) nightlySummary(ctx context.Context, runID string, owners []string, stages []NightlyStageResult) NightlyStageResult {
	if fail := d.nightlyInjectedFailure(ctx, runID, nightlyStageSummary, owners); fail != nil {
		return *fail
	}
	if len(owners) == 0 {
		return NightlyStageResult{Stage: nightlyStageSummary, Status: memory.LedgerNoData,
			Counts: map[string]any{"owners": 0}, Detail: "no banks for run summary"}
	}
	failedStages := 0
	for _, s := range stages {
		if s.Status == memory.LedgerFailed {
			failedStages++
		}
	}
	memoriesStored, episodesFolded, candidatesPromoted, memoriesMerged, generalizationsProposed := 0, 0, 0, 0, 0
	memoriesForgotten, memoriesPinned, memoriesContradicted := 0, 0, 0
	for _, s := range stages {
		switch s.Stage {
		case nightlyStageFold:
			memoriesStored += countNightly(s.Counts, "memories")
			episodesFolded += countNightly(s.Counts, "episodes")
		case nightlyStagePromote:
			candidatesPromoted += countNightly(s.Counts, "proposed")
		case nightlyStageMemoryConsolidate:
			memoriesMerged += countNightly(s.Counts, "merged")
			generalizationsProposed += countNightly(s.Counts, "proposed")
		}
	}
	failed := 0
	var firstErr string
	banks, openFailed, openErr := d.nightlyOpenBanks(owners)
	failed += openFailed
	if firstErr == "" {
		firstErr = openErr
	}
	for _, owner := range owners {
		bank, ok := banks[owner]
		if !ok {
			continue
		}
		forgotten, pinned, contradicted, countErr := nightlyMemorySummaryCounts(ctx, bank)
		if countErr != nil {
			failed++
			if firstErr == "" {
				firstErr = countErr.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageSummary, memory.LedgerFailed, map[string]any{}, "summary failed: "+countErr.Error())
			continue
		}
		memoriesForgotten += forgotten
		memoriesPinned += pinned
		memoriesContradicted += contradicted
		items, err := bank.Queue(ctx)
		if err != nil {
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
			nightlyRecordLedger(ctx, bank, runID, nightlyStageSummary, memory.LedgerFailed,
				map[string]any{}, "summary failed: "+err.Error())
			continue
		}
		st := memory.DescribeQueue(items, d.now())
		counts := map[string]any{
			"stages": len(stages) + 1, "failedStages": failedStages,
			"pending": st.Pending, "approved": st.Approved, "rejected": st.Rejected,
			"total": st.Total, "budget": st.Budget, "overBudget": st.OverBudget,
			"avgApprovalLatencyHours": st.AvgLatencyHrs, "oldestPendingAgeHours": st.OldestPendingAgeHrs,
			"memoriesStored": memoriesStored, "episodesFolded": episodesFolded, "candidatesPromoted": candidatesPromoted,
			"memoriesMerged": memoriesMerged, "generalizationsProposed": generalizationsProposed,
			"memoriesForgotten": memoriesForgotten, "memoriesPinned": memoriesPinned, "memoriesContradicted": memoriesContradicted,
		}
		detail := fmt.Sprintf("nightly %s: %d stages, %d failed; queue %d pending / %d total (budget %d)",
			runID, len(stages)+1, failedStages, st.Pending, st.Total, st.Budget)
		nightlyRecordLedger(ctx, bank, runID, nightlyStageSummary, memory.LedgerSuccess, counts, detail)
	}
	status := memory.LedgerSuccess
	detail := fmt.Sprintf("nightly %s: %d stages, %d failed", runID, len(stages)+1, failedStages)
	if failed > 0 {
		status = memory.LedgerFailed
		detail = fmt.Sprintf("summary partial: %s; %s", firstErr, detail)
	}
	return NightlyStageResult{Stage: nightlyStageSummary, Status: status,
		Counts: map[string]any{"owners": len(owners), "stages": len(stages) + 1, "failedStages": failedStages, "failed": failed, "memoriesStored": memoriesStored, "episodesFolded": episodesFolded, "candidatesPromoted": candidatesPromoted, "memoriesMerged": memoriesMerged, "generalizationsProposed": generalizationsProposed, "memoriesForgotten": memoriesForgotten, "memoriesPinned": memoriesPinned, "memoriesContradicted": memoriesContradicted},
		Detail: detail}
}

func countNightly(counts map[string]any, key string) int {
	switch n := counts[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// nightlyMemorySummaryCounts counts Memory state across one bank. Contradictions
// come from memory_contradicted ledger events; legacy contradicted rows also count.
func nightlyMemorySummaryCounts(ctx context.Context, bank memory.Memory) (forgotten, pinned, contradicted int, err error) {
	for _, status := range []string{memory.StatusActive, memory.StatusParked, memory.StatusSuperseded, memory.StatusForgotten} {
		items, e := bank.ListMemories(ctx, "", status, -1)
		if e != nil {
			return 0, 0, 0, e
		}
		if status == memory.StatusForgotten {
			forgotten = len(items)
		}
		for _, item := range items {
			if item.Pinned != 0 {
				pinned++
			}
		}
	}
	rows, err := bank.Ledger(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, row := range rows {
		if row.Stage != "memory_contradicted" && row.Stage != "contradicted" {
			continue
		}
		var counts map[string]any
		if json.Unmarshal([]byte(row.CountsJSON), &counts) == nil {
			count := countNightly(counts, "memories")
			if count == 0 {
				count = countNightly(counts, "count")
			}
			if count > 0 {
				contradicted += count
				continue
			}
		}
		contradicted++
	}
	return forgotten, pinned, contradicted, nil
}

// RunNightly runs the ordered nightly stages. Catch-up ingest uses the turn
// cursor; later failures remain visible without suppressing subsequent work.
func (d *Daemon) RunNightly(ctx context.Context) (NightlyResult, error) {
	d.nightlyMu.Lock()
	defer d.nightlyMu.Unlock()
	started := d.now()
	if !d.Idle(0) {
		return NightlyResult{StartedAt: started, Aborted: true, Error: "nightly: daemon busy"}, fmt.Errorf("nightly: daemon busy")
	}
	runID := newID()
	res := NightlyResult{RunID: runID, StartedAt: started}
	owners := d.nightlyOwners()
	snap := d.nightlySnapshot(ctx, runID, owners)
	res.Stages = append(res.Stages, snap)
	if snap.Status == memory.LedgerFailed {
		res.Aborted = true
		res.Error = snap.Detail
		return res, fmt.Errorf("nightly: %s", snap.Detail)
	}
	mined, mine := d.nightlyMine(ctx, runID, owners)
	res.Stages = append(res.Stages, mine)
	briefs, sessionOwner, distill := d.nightlyDistill(ctx, runID, owners, mined)
	res.Stages = append(res.Stages, distill)
	res.Stages = append(res.Stages, d.nightlyReembed(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlyFold(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlyConsolidateMemories(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlyConsolidate(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlyPromote(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlyPair(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlyProfile(ctx, runID, owners, briefs, sessionOwner))
	res.Stages = append(res.Stages, d.nightlyAnchors(ctx, runID, owners))
	res.Stages = append(res.Stages, d.nightlySummary(ctx, runID, owners, res.Stages))
	return res, nil
}

// nightlyDue compares the ledger's latest summary with the most recent local
// 03:00 boundary. A missed run stays due at any hour until summarized.
func nightlyDue(now, lastSummary time.Time) (bool, time.Time) {
	local := now.Local()
	cutoff := time.Date(local.Year(), local.Month(), local.Day(), 3, 0, 0, 0, local.Location())
	if local.Before(cutoff) {
		cutoff = cutoff.AddDate(0, 0, -1)
	}
	return lastSummary.IsZero() || lastSummary.Before(cutoff), cutoff
}

// nightlyMaybeRun checks the ledger-backed marker on every minute. Busy runs
// leave one skipped row per local day and remain due until an idle retry.
func (d *Daemon) nightlyMaybeRun(now time.Time) bool {
	owners := d.nightlyOwners()
	var lastSummary time.Time
	day := now.Local().Format("2006-01-02")
	skipped := false
	for _, owner := range owners {
		bank, err := d.profileBankFor(owner)
		if err != nil {
			continue
		}
		rows, err := bank.Ledger(context.Background())
		if err != nil {
			continue
		}
		for _, row := range rows {
			if row.Stage == nightlyStageSummary && row.CreatedAt.After(lastSummary) {
				lastSummary = row.CreatedAt
			}
			if row.Stage == "nightly_skipped_busy" && row.Detail == day {
				skipped = true
			}
		}
	}
	due, _ := nightlyDue(now, lastSummary)
	if !due {
		return false
	}
	if !d.Idle(0) {
		if !skipped {
			for _, owner := range owners {
				if bank, err := d.profileBankFor(owner); err == nil {
					nightlyRecordLedger(context.Background(), bank, "busy-"+day, "nightly_skipped_busy", memory.LedgerSuccess, map[string]any{"day": day}, day)
				}
			}
		}
		return false
	}
	res, _ := d.RunNightly(context.Background())
	return !(res.Aborted && len(res.Stages) == 0)
}

// StartNightlyLoop checks the ledger-backed due condition every minute; a
// missed or busy run therefore starts at the next idle opportunity.
func (d *Daemon) StartNightlyLoop(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			d.nightlyMaybeRun(now)
		}
	}
}

// nightlyAdminOf resolves the signed-in admin behind the on-demand nightly
// run. The job snapshots every bank and mines every Session, so triggering it
// is admin-only like the other host-global switches; ordinary Users and the
// machine token are refused.
func (d *Daemon) nightlyAdminOf(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := d.memoryUserOf(w, r)
	if !ok {
		return User{}, false
	}
	if !user.Admin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return User{}, false
	}
	return user, true
}

// handleNightly serves POST /api/memory/nightly: the admin on-demand run from
// the Memory page. It runs only when idle: a busy Daemon answers 409 with no
// ledger write. The job runs detached from the request context, so a client
// disconnect never cancels a run midway.
func (d *Daemon) handleNightly(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.nightlyAdminOf(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := d.RunNightly(context.WithoutCancel(r.Context()))
	if err != nil && res.Aborted && len(res.Stages) == 0 {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, redactServedDiagnostics(res))
}
