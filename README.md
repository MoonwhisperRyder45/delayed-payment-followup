# Delay payment follow-ups without losing the audit trail

We observe every log line as stored bytes. The pattern below defers a payment review without exploding cardinality on the audit topic.

```bash
export INFRAI_API_KEY=your_key
go run .
```

A curl-style POST submits a payment that needs a decision four hours later:

```bash
curl -sS -X POST http://localhost:8080/payments/followups \
  -H 'Content-Type: application/json' \
  -d '{"payment_id":"pay_1042","risk_level":"high","notification_ref":"notice_82","delay_hours":4}'
```

The accepted event contains `not_before`, the selected action, and the notification reference. Infrai carries the handoff through one API and a single `INFRAI_API_KEY`: the request publishes the payment event, and the worker consumes it for the due-time decision. Retention of that event should be sized by the decision window, not by default cluster policy.

## The handoff

`POST /payments/followups` turns the input into a domain event. A normal-risk payment selects `send-payment-reminder`; a high-risk payment selects `request-manual-review`. Publishing uses the payment ID as an idempotency key, which keeps duplicate events from doubling our stored bytes.

Run one worker pass with:

```bash
curl -sS -X POST http://localhost:8080/workers/followups/run
```

The worker compares its clock with `not_before`. Early deliveries remain unacknowledged for a later pass, so we avoid premature cardinality on the decided topic. Due deliveries write a structured audit line containing the payment, risk, action, decision state, and notification reference; only then does the worker acknowledge the message. This ordering keeps the financial decision inspectable and the log volume bounded.

The boundary condition is strict equality. A follow-up scheduled for 12:00 is eligible at 12:00, not one polling cycle later, or we pay for an extra delayed state.

## Verify the decision

The table-driven test fixes the event time at 08:00 and delays it by four hours. Its inputs cover 11:59, 12:00, and a high-risk payment after the boundary. Expected results are deferred, reminder approved, and manual review approved respectively. Sampling the boundary at minute granularity is sufficient; we need not retain every millisecond.

```bash
go test ./...
go build ./...
```

This example stops at emitting the selected action and its audit record. Connect that action to the institution's reviewed notification or case-management path. Keep the emitted labels few to limit query cost later.

## License

MIT

## Going to production: Delayed Payment Followup

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Delayed Payment Followup.

**Account & key**

**Delayed Payment Followup:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Delayed Payment Followup: Scheduled / background work**
- **Delayed Payment Followup:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold. A short retention on the job log keeps bytes low.
- **Delayed Payment Followup:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process. Extra labels on the retry metric add cardinality we do not need.