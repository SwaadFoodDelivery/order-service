# Swaad order-service — read-only gRPC integration

Implemented: authenticated client-owned `order.v1.OrderService/GetOrder`, reading
persisted order and item snapshots through a repeatable-read, read-only PostgreSQL
transaction. Integer minor units are authoritative (INR paise). The service is
actually registered using the generated, version-pinned protobuf module.

Not implemented here: PlaceOrder, GetUserOrders, CancelOrder, UpdateOrderStatus,
GetOrderTracking. They explicitly return Unimplemented, not mock success.
The existing backend still owns checkout, payment/delivery mutations, migrations
and the single orders database. This is the first extraction slice, not a full
microservice migration. No Redis/Kafka connection or migrations run at startup.
Legacy packages/migration files are retained as historical scaffold; do not run
this repository's copied migrations against the current application database.

## Local startup

Use Go 1.24.5+ and backend schema29. Provision a SELECT-only role with USAGE on
public and SELECT on orders, order_items and payments in the dedicated database.
Keep schema creation and seed commands with the backend repository.

```sh
APP_ENV=development ORDER_GRPC_ADDR=127.0.0.1:50051 \
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

## Verification

```sh
go test -race ./...
go vet ./...
ORDER_RPC_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/swaad_grpc_test_20260912?sslmode=disable' \
  go test -race ./internal/grpc/server -run TestGetOrderPostgres -count=1 -v
```

The integration command requires the backend's actual schema29 and fictional
demo catalog. It only permits `swaad_grpc_test_*` databases, creates new fictional
records, and verifies actual bufconn→SQL reads using a temporary SELECT-only role.
It covers ownership misses, partitioned composite item keys, stored price/name
snapshots, .05 exact amounts, successful payment priority, and unchanged records.
Fictional records remain in the disposable database; the temporary role is removed.
Do not point tests at shared or production data. Unit tests also cover actual
loopback sockets, missing/wrong/duplicate credentials, role/UUID denial,
deadlines/cancellation, panic sanitization and all unimplemented RPCs.

Cross-repository compatibility: proto PR1
`v0.0.0-20260912173032-f946f9d3345e`, backend PR18. Paired backend HTTP→actual
order-service→PostGIS validation and independent review remain release gates.
Rollback by disabling the new backend read route and stopping this process; no
data migration or duplicated order records need to be undone.
