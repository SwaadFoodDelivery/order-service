# Swaad order-service — read-only gRPC integration

Implemented: authenticated client-owned `order.v1.OrderService/GetOrder`, reading
persisted order and item snapshots through a repeatable-read, read-only PostgreSQL
transaction. Integer minor units are authoritative (INR paise). The service is
actually registered using the generated, version-pinned protobuf module.

Also implemented: `GetUserOrders` returns bounded order summaries with signed
keyset pagination. It reads orders, current restaurant names (including inactive
restaurants), and optional delivery status in one joined SELECT inside a read-only,
repeatable-read transaction. Delivery joins include both order_id and created_at.
It does not hydrate items or payment attempts. Summary payment_status stays empty;
payment_method is the stored order field. Deprecated total stays zero, including
nonempty pages; no count query is performed. Deprecated double money fields remain
populated from authoritative integer minor units for compatibility.

Not implemented here: PlaceOrder, CancelOrder, UpdateOrderStatus,
GetOrderTracking. They explicitly return Unimplemented, not mock success.
The existing backend still owns checkout, payment/delivery mutations, migrations
and the single orders database. This is the first extraction slice, not a full
microservice migration. No Redis/Kafka connection or migrations run at startup.
Legacy packages/migration files are retained as historical scaffold; do not run
this repository's copied migrations against the current application database.

## Local startup

Use Go 1.24.5+ and backend schema29. Provision a SELECT-only role with USAGE on
public and SELECT on orders, order_items, payments, restaurants and deliveries in
the dedicated database. Do not grant INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES,
TRIGGER, table ownership or a privileged inherited role. The runtime role must not
own these tables. An operator must extend an existing reader's grants; the service
never grants privileges itself.
Keep schema creation and seed commands with the backend repository.

```sh
APP_ENV=development ORDER_GRPC_ADDR=127.0.0.1:15052 \
ORDER_GRPC_SERVICE_KEY='<private-at-least-32-character-key>' \
POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=5432 \
POSTGRES_USER='<read-only-role>' POSTGRES_PASSWORD='<private-local-password>' \
POSTGRES_DB=swaad_grpc_test_20260912 POSTGRES_SSLMODE=disable \
go run ./cmd/server
```

Replace placeholders locally; never paste real credentials into Git or chat.
Configure the same key on backend PR18 with ORDER_GRPC_REQUIRED=true and
ORDER_GRPC_ADDR matching this loopback listener. Backend `GET /api/v1/orders/:id`
then invokes this service; order creation remains in the backend.
For the paired list feature use a new reader role and port 15052. The existing
GetOrder runtime on 15051 belongs to the completed feature and must not be restarted
as part of this rollout.

Plaintext RPC is allowed only with explicit development/test and numeric loopback
IPs. Public, DNS and production targets fail configuration validation until TLS
is implemented. Service key metadata is authenticated first; requester UUID and
client role are then validated and SQL filters by owner. Foreign and missing
orders are indistinguishable NotFound. No admin or arbitrary role bypass exists.
Configure ORDER_GRPC_TIMEOUT_MS from 1–2000 (default2000); caller deadlines can
shorten it. Dependency failures do not turn into successful reads. Internal SQL,
panic details and credentials are not logged or returned in RPC error messages.

Database startup is checked before listening. Connections default to read-only,
statements have a two-second limit, the pool is bounded, and shutdown is graceful
with a three-second cap. Use SELECT-only credentials as a second boundary.

## List pagination contract

Authenticate `x-order-service-key` before validating requests. `user_id` must be a
nonzero canonical-format UUID, `requester_role` must be exactly `client`, and limit
must be 1–50; invalid limits are rejected, never defaulted or clamped. The first
page uses an empty cursor. Every SELECT includes the requester user_id filter and
sorts by created_at DESC, order_id DESC. Subsequent pages add a strict tuple `<`
filter. Fetch limit+1, return at most limit, and emit a next_cursor from the last
returned row only when the extra row exists. Empty and terminal pages have no cursor.

Cursors are opaque API tokens, authenticated with HMAC-SHA256 using a key derived
from the service key and the domain `swaad/order-service/GetUserOrders/cursor/v1`.
They are not encrypted. The versioned payload binds the requester UUID to the exact
UTC RFC3339Nano timestamp (PostgreSQL microseconds preserved) and order UUID. Inputs
over 1024 bytes, invalid encodings, modified signatures, unsupported versions, wrong
owners and malformed positions return InvalidArgument before repository access.
Service-key rotation invalidates outstanding cursors. There is no cursor expiry.

A cursor is a position, not a snapshot across requests. New rows above that position
will appear on a refreshed first page, not on continued pages. Backfilled rows below
it can appear on later pages. Current restaurant names and delivery statuses can also
change between requests. Each individual page has its own consistent database view.

## Verification

```sh
go test -race ./...
go vet ./...
ORDER_RPC_TEST_DATABASE_URL='<private-admin-URL-for-swaad_grpc_test_20260912>' \
  go test -race ./internal/grpc/server -run 'Test(GetOrder|GetUserOrders)Postgres' -count=1 -v
```

The integration command requires the backend's actual schema29 and fictional
demo catalog. It only permits `swaad_grpc_test_*` databases, creates new fictional
records, and verifies actual bufconn→SQL reads using a temporary SELECT-only role.
It covers ownership misses, partitioned composite item keys, stored price/name
snapshots, .05 exact amounts, successful payment priority, and unchanged records.
The list suite additionally covers UUID tie-breaking, microsecond boundaries,
limit+1 and exact terminal pages, inserts between pages, inactive/current restaurant
names, missing/composite deliveries, foreign cursor rejection and SELECT-only grants
on all five tables. Fictional records remain in the disposable database; temporary
roles are removed. Notify root before using the dedicated database to serialize
fixture work. Never provision a database or run migrations from these tests.
Do not point tests at shared or production data. Unit tests also cover actual
loopback sockets, missing/wrong/duplicate credentials, role/UUID denial,
deadlines/cancellation, panic sanitization and all unimplemented RPCs.

Cross-repository compatibility: proto list contract
`v0.0.0-20260913090154-dc2e8c6d6c1d`, additive to proto PR1 and backend PR18.
The list service change stacks on `codex/order-grpc-foundation`. Paired backend HTTP→actual
order-service→PostGIS validation and independent review remain release gates.
Rollback by disabling the new backend read route and stopping this process; no
data migration or duplicated order records need to be undone.
