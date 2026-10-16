---
status: accepted
---

# Memories are automatic; durable shared guidance remains reviewed

## Decision

Store grounded, redacted Memories automatically in the bank of the User who sent the source Event. Memories are private recall material: a User may forget, park, pin, edit or promote one, but ordinary extraction does not wait for approval. Rules, Profile patches and Project notes continue to require human approval before they become durable shared guidance or are written into a repository.

## Context

Requiring approval for every extracted observation left the queue unnoticed and made Memory ineffective. Memories are short, source-quoted and constrained to the sender's private bank; automatic storage improves retention without granting an agent permission to change reviewed instructions or repository files. Promotion from a Memory creates a pending Rule proposal rather than an approved Rule.

## Rejected alternative

Approve every Memory before storage or recall. This would preserve a uniform approval gate, but repeats the queue bottleneck that caused Memory to retain almost nothing and makes low-risk private recall dependent on a human reviewing every item.

## Consequences

- Users may see and correct automatic Memories without approving them first; they can forget or park unwanted items.
- Grounding, redaction, sender ownership and soft-forget rules remain mandatory.
- A Memory never directly changes the Profile, creates an approved Rule, or edits Project notes. Those transitions retain explicit human review.
- The Memory page exposes production proxies so retention, forgetting, promotion, injection and cost can be monitored; daily Memory model cost above $0.05 is an alert.
- Automatic extraction increases the amount of private, redacted user text stored locally. The extractor/embedder egress remains disclosed in Memory settings.
