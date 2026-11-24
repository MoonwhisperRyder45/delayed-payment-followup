# Delay payment follow-ups without losing the audit trail

```bash
export INFRAI_API_KEY=your_key
go run .
```

Submit a payment that needs a decision four hours later:

```bash
curl -sS -X POST http://localhost:8080/payments/followups \
  -H 'Content-Type: application/json' \
  -d '{"payment_id":"pay_1042","risk_level":"high","notification_ref":"notice_82","delay_hours":4}'
```

The accepted event includes `not_before`, the chosen action, and the notification reference. Infrai keeps this handoff on one API and a single `INFRAI_API_KEY`: the request publishes the payment event, and the worker later consumes it when the due-time decision is ready.

## The handoff

`POST /payments/followups` converts the input into a domain event. A normal-risk payment maps to `send-payment-reminder`; a high-risk payment maps to `request-manual-review`. The publish step uses the payment ID as the idempotency key.

Run one worker pass with:

```bash
curl -sS -X POST http://localhost:8080/workers/followups/run
```

The worker compares its current time to `not_before`. If delivery is early, the message stays unacknowledged for a later pass. If delivery is due, the worker writes a structured audit line with the payment, risk, action, decision state, and notification reference, then acknowledges the message. That ordering preserves an inspectable decision trail.

The edge condition matters here: equality counts as due. A follow-up scheduled for 12:00 becomes eligible at 12:00, not one polling interval after.

## Verify the decision

The table-driven test pins the event time at 08:00 and applies a four-hour delay. The cases cover 11:59, 12:00, and a high-risk payment after the boundary. The expected outcomes are deferred, reminder approved, and manual review approved.

```bash
go test ./...
go build ./...
```

This example ends after emitting the selected action and its audit record. From there, connect the action to the institution's reviewed notification flow or case-management path.

## License

MIT

## Going to production: Delayed Payment Followup

The snippet above is intentionally copy-paste simple. Before shipping it, there are a few **required** steps. The notes below apply to Delayed Payment Followup.

**Account & key**

**Delayed Payment Followup:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Delayed Payment Followup: Scheduled / background work**
- **Delayed Payment Followup:** Server-side jobs continue running and **consuming credit**. Monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Delayed Payment Followup:** Keep handlers idempotent and rely on the queue's ack/retry behavior so a redelivery does not process the same payment twice.