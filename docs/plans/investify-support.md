# Investify consolidated support implementation plan

Prepared 30 September 2026. The goal is one useful automated WhatsApp answer per customer per month, covering all pending questions, with further help moved to email. Routine operation must require no review queue or daily manual decisions.

## Audit findings

The review covered about 32 conversations, including nine older June–August conversations to reduce the bias from the recent chart and announcement incident. This is a qualitative sample, not a random estimate of topic percentages. Raw customer messages and identifiers are intentionally excluded from this document.

| Pattern observed | Consequence | Change |
| --- | --- | --- |
| Login and registration problems, including existing accounts and reset emails | Greetings and requests for device details sometimes precede useful recovery steps | Combine related account questions and provide relevant recovery steps together |
| Short consecutive messages and follow-up acknowledgements | Duplicate advice and replies to thanks or OK | Accumulate pending messages and classify the whole conversation |
| Brokerage and Investify account confusion | Customers can mistake portfolio entries for real trading | Explain separate accounts and manual trade tracking when relevant |
| Web and cross-device questions | An old answer denied desktop availability | Add verified web availability and shared-account guidance |
| Keyword matches on ordinary words such as information | A generic introduction can replace the answer to a bug report | Bypass keywords and greeting flows in consolidated mode |
| Repeated requests for screenshots, versions or steps already supplied | Longer conversations without progress | Preserve supplied facts and attempted steps in the question summary |
| Unverified app versions, escalation promises and claims of human identity | Customers receive unsupported assurances | Use approved knowledge; acknowledge uncertainty and automation honestly |
| Recent chart and announcement loading problems | Recent conversations overrepresent an unusual incident | Keep incidents separately dated and expiring; evaluate evergreen topics independently |

The live AI settings use OpenAI `gpt-4.1-nano`, a 1,000-token response limit and an empty system prompt. Current behaviour therefore depends heavily on the embedded knowledge and chatbot orchestration. The code returns after sending a new-session greeting, before answering the first substantive message. Keyword replies can also bypass AI entirely. Changing only the prompt will not solve either problem.

## What the volume evidence establishes

The earlier contact export contained 7,527 unique contacts. New contacts were 911 in June, 1,011 in July, 883 in August and 1,009 in September. Meta Insights showed 4,197 sends and 4,196 deliveries over its selected last-30-days period.

The deployed database report now separates incoming customers, customers successfully replied to, and successful sends. Calendar months use PKT. September is partial through deployment on 30 September.

| Month | Customers messaging | Customers replied to | Successful sends |
| --- | ---: | ---: | ---: |
| April 2026 | 1,213 | 1,202 | 3,941 |
| May 2026 | 892 | 881 | 2,603 |
| June 2026 | 972 | 957 | 3,222 |
| July 2026 | 1,091 | 1,077 | 3,258 |
| August 2026 | 957 | 946 | 2,747 |
| September 2026 | 1,107 | 1,092 | 4,242 |

Monthly substantive demand is not yet measured: old greetings and keyword replies inflated the number of customers replied to. The classifier will expose skipped introductions and genuine requests separately. Several months exceeded 1,000 unique recipients, so the hard cap is necessary even with one automated response per customer. Current-month historical sends count on activation; budget accounting does not start at zero.

## Confirmed product knowledge and remaining uncertainty

The official [desktop page](https://www.investify.pk/investify-for-pc) confirms a browser app and the same account across web, Android and iOS, with portfolio and watchlist sync. The [FAQ](https://www.investify.pk/faq) describes portfolio entries and subscription management. The [contact page](https://www.investify.pk/contact) lists `support@investify.pk` for product support.

The revised embedded knowledge removes unsupported exact version numbers, universal card compatibility, fixed ad frequency and release promises. Multiple portfolios, web subscription entitlements and cross-platform purchase restoration need verified product evidence before the AI asserts them. Until then, it should give an honest email handoff. Public contact copy currently encourages WhatsApp follow-up; updating that copy belongs in the Investify website project.

The older [refund policy](https://www.investify.pk/refundpolicy) uses `contact@investify.pk` and describes direct refund review, while the newer FAQ points to app-store handling and the contact page routes product support to `support@investify.pk`. These pages should be reconciled in the website project. The bot should identify the purchase platform and avoid promising a refund or quoting a turnaround time.

## Agreed operating rules

1. **Customer identity:** use the canonical WhatsApp customer identity within the organization, shared across sessions. Normalize phone numbers and use the existing contact identity; do not count each chat session as a new customer.
2. **Normal schedule:** prepare answers ahead of the daily 09:00 Asia/Karachi batch. The server timezone must not affect the schedule.
3. **Expiry safeguard:** send at the earlier of the next batch or eight minutes before the latest valid inbound event plus 24 hours. For an expiry at 09:00, that is 08:52; an 08:15 expiry needs 08:07. A fixed 08:52 sweep alone is insufficient. Recheck immediately before sending.
4. **Monthly customer allowance:** at most one automated send per customer per PKT calendar month. New messages after it stay visible but do not trigger another reply. Starting a new month does not resurrect stale questions or closed service windows.
5. **Manual replies:** remain available, count against the business-number budget and suppress automation for that customer for the rest of the month after successful acceptance. An in-progress manual send temporarily holds automation. A definitive failure restores eligibility; an uncertain outcome remains held to avoid duplicates.
6. **Global budget:** reserve at most 1,000 service sends per business number per billing month, including manual sends. Pause automation for the rest of the month at the cap. Manual exemption from the customer allowance is not permission to exceed the global cap. Show warnings at 800 and 950.
7. **No extra sends:** disable separate greetings, keyword responses, broker PDFs, out-of-hours messages, reminders and automatic fallback chatter for accounts using consolidated mode. Block automatic paid-template workarounds and audit campaign/API paths for bypasses.
8. **No routine review:** use automatic classification, preparation, bounded retries and an email handoff for established support intent that cannot be answered. Do not create a daily approval list.

Meta's billing-month timezone must be confirmed from authoritative account/API documentation before configuring the budget reset. The PKT customer allowance and Meta billing bucket are separate concepts. Accepted, in-flight and uncertain sends reserve capacity; delivery webhooks reconcile it conservatively. Do not promise zero Meta charges if another application sends outside Whatomate or paid templates are used independently.

## Two AI stages

### Queue classification and question summary

Input: pending incoming messages, relevant earlier conversation, direction and stable message IDs, media metadata, and already attempted steps. Keep customer content in an untrusted user-data envelope. Use the original messages for evidence; earlier bot answers do not establish product facts.

Output: validated JSON containing `answer`, `skip` or `email`, reason, language, summary and an array of questions. Each question includes its category, source message IDs and attempted steps. Only pending incoming messages can justify new work. Persist the input revision and prompt version with the decision.

Skip empty iOS/Android introductions, greetings, acknowledgements, emoji, spam and fully resolved requests. An introduction with an actual problem appended remains eligible. A final thanks does not discard earlier unanswered questions. Retain unrelated questions separately and combine duplicates without losing meaning. Unknown images or audio must not be described as though their contents were read.

### Consolidated answer

Input: validated classification, original relevant messages, approved product knowledge and any explicitly active incident facts. Output: one answer covering every unresolved topic, in English, Roman Urdu or Urdu script as appropriate.

Use a maximum 3,000-character answer body and append the application-owned email footer, keeping the complete message below 3,500 characters. WhatsApp text supports up to 4,096 characters; the smaller application limit provides room for formatting. Reject or regenerate oversized answers, then use one complete email handoff if necessary. Never truncate midway through instructions or split into several messages.

Suggested footer: “To keep WhatsApp support sustainable, automated replies are limited to one per month. For further help or questions, please email support@investify.pk.” This describes Investify's policy without claiming Meta forbids manual support.

Keep the current provider/model initially and test it on anonymized fixtures. Configure separate token budgets for classification and answers so the old 1,000-token setting cannot silently truncate the new output. No model can change send limits or override manual suppression.

## Email call to action

Use the direct address `support@investify.pk` in the same plain-text reply. On 1 October the owner removed the support landing-page URL from outgoing replies. Do not include that URL or attach a URL button. Existing queued answers are re-prepared and the sender also replaces the legacy URL before dispatch. Already sent messages remain part of the send ledger and are never resent.

## Implementation sequence

### 1 Knowledge and preparation foundation

- Replace outdated embedded knowledge with sourced platform facts and bounded troubleshooting guidance.
- Add versioned, separate classifier and answer prompts.
- Add a provider-independent preparation function with strict JSON/evidence validation, whole-conversation noise handling and an automatic answer-failure handoff.
- Add pure timing and PKT month functions, plus regression tests for expiry and monthly boundaries.
- Preserve disabled keyword rules across application restarts.

### 2 Durable queue and database records

- Add a daily-support setting scoped to organization/account, defaulting off until the complete send path is ready.
- Persist one conversation job per organization/business number/customer, with source event timestamps, pending message range, revision, due time, decision, summary, state, retries and claim lease.
- Persist customer-month suppression and send reservations with uniqueness constraints; persist business-number billing buckets and send-attempt states.
- Save valid Meta event timestamps rather than using webhook arrival time to extend a reply window. Deduplicate by message ID and ignore out-of-order timestamp regressions. Invalid timestamps must not grant a fresh window.
- Add a read-only monthly metrics query and status endpoint. Keep raw customer content out of application logs and public reports.

### 3 Incoming processing and classification worker

- Save the incoming message first and only enqueue after a confirmed insert. In consolidated mode, branch before legacy greeting, keyword, flow and out-of-hours processing.
- Debounce incoming fragments briefly, then classify the accumulated pending messages. New content invalidates an older prepared answer; a final send check must compare revisions.
- Preserve assigned-agent and active-transfer suppression. Manual takeover must prevent a worker from sending a stale answer.
- Use the configured AI provider through a cancellable adapter with session history disabled; the structured transcript already supplies history once.
- If classification is unavailable or malformed, retry within a bounded deadline. Never interpret failure as authorization to send. If intent remains unknown at expiry, record the failure and stop; do not create a human review queue or send outside the window.
- Preserve every pending message up to 200 messages / 64 KB of text. Larger transcripts stop with a visible preparation error rather than silently dropping questions; automatic compaction remains a future improvement.

### 4 Dispatch and quota enforcement

- Prepare answers before the batch and run a lightweight PostgreSQL-backed worker at least every 30 seconds. Claim work with a lease and process expiry cases first; do not hold database locks while calling AI.
- Inside a short transaction, lock the customer and business-number budget, recheck current revision/suppression/window, and reserve capacity before calling WhatsApp.
- Enforce the same policy in the unified outgoing sender and any direct campaign/API send paths. Reject disallowed sends with a clear API/UI error.
- Record acceptance with WhatsApp message ID. A crash or timeout after dispatch creates an uncertain reservation; do not retry blindly. Database uniqueness alone cannot make an external send exactly once.
- Return synchronous send errors accurately. The current unified sender finalizes a failed synchronous send but still returns a nil error; the new worker must not mistake that for success.
- Reconcile definitive failures and delivery events idempotently. Do not refund an uncertain reservation just because a lease expired.
- Keep a prepared job's stored due time. Recalculating “next 09:00” after a worker wakes at 09:00:01 must not defer the batch to tomorrow.

### 5 Settings and operational visibility

- Add consolidated-mode configuration, fixed PKT schedule description, monthly allowance, cap and email destination to chatbot settings.
- Show pending, classified, sent, skipped, suppressed, expired and uncertain counts; next send time; remaining quota; and the active prompt/knowledge version.
- Show reasons and summaries for troubleshooting, with no approval action required.
- Add the email landing page and use a URL button only when its full body fits.
- Make quota exhaustion visible and auto-reset the business budget at the verified billing boundary. New customer messages in the new month can become eligible again.

### 6 Verification and release

- Add PostgreSQL integration tests for concurrent workers, duplicate webhooks, restarts, manual/automatic races, cap exhaustion, rollover, out-of-order inbound events and ambiguous Meta outcomes.
- Test English, Roman Urdu and Urdu; boilerplate alone versus appended questions; unrelated questions; acknowledgements after unanswered requests; partial/full resolution; screenshots; old false bot claims; and prompt-injection attempts in messages.
- Evaluate the two prompts against anonymized fixtures using the configured provider. Unit tests with a fake provider verify orchestration, not actual model quality.
- Run backend race tests, frontend type checks/build and relevant UI tests. Direct rollout is approved; a draft-only operating period is not required.
- Before enabling, capture existing settings for rollback, calculate current-month historical sends and suppression, and ensure every sending path honors the policy. Do not give the account a fresh 1,000-send allowance by ignoring messages already sent this month.
- Build the release image in GitHub Actions and deploy the exact image through the existing release branch/CapRover workflow. The low-capacity server pulls the built image.
- Confirm the release commit/image in the running app, health, migration success, enabled settings, stopped legacy responses, queue worker activity and quota counters. A failed documentation workflow is separate from app deployment; a green build alone does not prove the running deployment changed.
- Review outcomes after a week of real operation: unique recipients, skips by reason, missed questions, fallback rate, delivery failures, expired cases and knowledge gaps. Update versioned knowledge/prompts based on evidence.

## Current implementation status

Implemented: PostgreSQL queue and leases, two-stage AI preparation, revision checks, monthly customer suppression, shared phone-number budget reservations, manual-send reconciliation, expiry protection, paused collection, queue dashboard, monthly customer report, synthetic prompt checks, and the email landing page. Existing keyword/greeting and campaign paths are blocked while consolidated mode is active.

Verification includes race-enabled concurrency and failure tests against isolated PostgreSQL/Redis, the frontend test suite, type checking and production build. Production activation and configured-model checks are performed after the release image is deployed. The first normal batch is the next 09:00 PKT after activation; expiry protection can send earlier when necessary.

Operational limits: AI currently uses the existing OpenAI configuration. Media is represented as metadata, so the assistant must ask for details by email rather than pretend to inspect it. Oversized transcripts fail visibly without truncation. Uncertain WhatsApp sends retain their capacity and customer suppression to prevent duplicate replies.

## Deployment verification — 30 September 2026

- Application code `77b5276` was pushed to main and release. GitHub Actions run [36686055308](https://github.com/rafiqlightwala/whatomate/actions/runs/36686055308) completed successfully. The live health endpoint reports support version `2026-09-30.5`.
- Consolidated support is enabled. Meta's account timezone was verified as Asia/Karachi. The first normal batch is 1 October 2026 at 09:00 PKT. September's existing 4,242 sends remain counted; the new billing month starts at midnight PKT.
- Queue inspection is available at [Chatbot overview](https://wa.recubetech.com/chatbot). It shows states, due dates, prepared answers, monthly customer counts and prompt checks. The email landing page is live at [/support/email](https://wa.recubetech.com/support/email).
- A genuine new incoming conversation appeared in the database queue and was skipped as a non-substantive introduction. The enabled policy persisted through the subsequent application deployment.
- Full backend tests passed. Focused race tests covered quota contention, customer contention, rejected/uncertain sends, stale revisions, out-of-order capture, urgent expiry during activation, provider diagnostics and structured-output requests. Frontend tests, type checking and production build passed.
- Initial live-model tests found weaknesses in resolved-request handling, question grouping and language classification. Prompts were strengthened and classification now uses strict structured outputs. Subsequent tests became blocked by repeated OpenAI HTTP 429 responses, including on the original model. The responses did not expose a recognized provider error code, so billing exhaustion versus rate limiting is unconfirmed.
- The original configured model, gpt-4.1-nano, was restored. Collection and bounded send safeguards remain active. Provider preparation failures retry after 2, 10 and then 30 minutes while the reply window remains open. No synthetic test sent a customer a WhatsApp message.

**Outstanding verification:** restore working OpenAI access and rerun the live prompt checks, then review prepared answers before the first batch. Deployment and queue capture are verified; AI reply readiness is not yet confirmed. The OpenAI account's quota/billing or access needs checking outside Whatomate.


## First batch audit and repair — 1 October 2026

The first audit after 09:00 PKT found 45 captured conversations and zero October sends. OpenAI had begun returning prepared replies, but a retry-accounting defect treated accumulated preparation failures as rejected WhatsApp sends. Prepared answers were incorrectly skipped, including jobs due the next day.

The repair separates AI preparation failures from definitive automatic send rejections. Successful preparation resets the AI counter. Three actual rejected automatic sends enter a visible `send_failed` state; failed manual sends do not consume those attempts. Targeted recovery rechecks unsent decisions from the affected prompt versions, excludes every job with a dispatch reservation, preserves due dates and increments revision before preparing again. Normal window, human takeover, customer-month and number-budget guards still apply.

The classifier now explicitly preserves a vague app complaint as support intent and describes the customer's need rather than inventing an agent follow-up question. The approved knowledge no longer contains a historical loading-incident anecdote that the model could repeat as a current diagnosis. Prompt/knowledge version is `2026-10-01.1`. Sending was paused during repair and real-model checks; capture continues.

A further feature-request spot check exposed an unsupported claim that custom stock alerts do not exist. The official FAQ confirms general market/portfolio/news notifications; individual price thresholds remain unconfirmed. Knowledge version `2026-10-01.2` makes that distinction explicit and re-prepares only unsent affected answers. An eighth live fixture covers feature suggestions. Two replies were accepted before this brief second pause; those reservations are excluded from re-preparation.

The owner requested removal of the email landing-page URL after the catch-up batch accepted 31 replies. Automation was paused immediately for version `2026-10-01.3`: new replies use plain text and the direct support email, with no landing-page link or button. A final send-time cleaner protects previously prepared bodies, while sent reservations remain excluded from recovery.


## Final live verification — 1 October 2026

- Deployed application commit `548baec`; [release run 36822189652](https://github.com/rafiqlightwala/whatomate/actions/runs/36822189652) succeeded. Health reports `2026-10-01.3` and the policy is active.
- WhatsApp accepted 31 catch-up replies. The October number budget is 31/1,000. Ten empty introductions/greetings were skipped; three eligible chats are ready for 2 October at 09:00 PKT. There are no pending preparation errors, failed sends, uncertain sends or expired jobs in the final snapshot.
- One conversation remains assigned to a human. One already-replied customer sent a new follow-up; the monthly guard suppressed a second automated response. This changes that customer's job state from sent to suppressed while preserving the original send and budget.
- Eight live-model fixtures passed on gpt-4.1-mini before the URL removal. Focused race tests after removal confirmed both newly generated output and previously persisted replies cannot dispatch the legacy URL, and the sender uses plain text. A real future login reply was inspected in the live UI and contains the direct support email without the landing-page URL or button.
- The 31 earlier replies had already included the landing-page link when the owner requested removal. They remain recorded and were not resent. Only unsent jobs were re-prepared under version 2026-10-01.3.
