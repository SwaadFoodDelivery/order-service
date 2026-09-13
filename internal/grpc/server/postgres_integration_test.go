package server_test

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	orderpb "github.com/SwaadFoodDelivery/proto/order"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"order-service/internal/services/order/repository"
)

// Uses the authoritative backend schema/seed, never this repo's legacy migrations.
// Committed fictional fixtures are retained only in the guarded disposable DB.
func TestGetOrderPostgres(t *testing.T) {
	dsn := os.Getenv("ORDER_RPC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORDER_RPC_TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || !regexp.MustCompile("^/swaad_grpc_test_[a-z0-9_]+$").MatchString(u.Path) {
		t.Fatal("requires dedicated swaad_grpc_test_* database")
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.Get(&version, "SELECT version FROM schema_migrations WHERE NOT dirty"); err != nil || version != 29 {
		t.Fatal("requires clean backend schema29")
	}
	owner, other := uuid.New(), uuid.New()
	orderID, foreignID := uuid.New(), uuid.New()
	created := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	old := created.Add(-time.Second)
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, uid := range []uuid.UUID{owner, other} {
		if _, err := tx.Exec("INSERT INTO users(user_id,phone,name,role,account_status,onboarding_complete) VALUES($1,$2,'Fictional RPC test','client','active',true)", uid, uid.String()[:14]); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range []struct {
		id, user uuid.UUID
		at       time.Time
		total    string
	}{
		{orderID, owner, old, "99.99"}, {orderID, owner, created, "10.05"}, {foreignID, other, created, "12.00"},
	} {
		_, err := tx.Exec("INSERT INTO orders(order_id,created_at,user_id,restaurant_id,status,subtotal,taxes,delivery_fee,total_amount,payment_method) VALUES($1,$2,$3,'10000000-0000-4000-8000-000000000001','confirmed',$4,0,0,$4,'upi')", fixture.id, fixture.at, fixture.user, fixture.total)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		at          time.Time
		name, price string
	}{
		{old, "Old snapshot must not leak", "99.99"}, {created, "Stored snapshot, not current menu", "0.05"},
	} {
		_, err := tx.Exec("INSERT INTO order_items(order_id,order_created_at,item_id,item_name_snapshot,item_price_snapshot,quantity,line_total) VALUES($1,$2,'40000000-0000-4000-8000-000000000001',$3,$4,1,$4)", orderID, item.at, item.name, item.price)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, payment := range []struct {
		at    time.Time
		state string
	}{{created, "success"}, {created.Add(time.Second), "failed"}} {
		_, err := tx.Exec("INSERT INTO payments(order_id,order_created_at,user_id,amount,status,idempotency_key,created_at) VALUES($1,$2,$3,10.05,$4,$5,$6)", orderID, created, owner, payment.state, uuid.NewString(), payment.at)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// A separate non-login role owns nothing and gets only the three required reads.
	// Fixture connection remains admin; repository connection runs as this role.
	role := "swaad_rpc_reader_" + uuid.New().String()[:8]
	if _, err := db.Exec("CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec("REVOKE SELECT ON orders, order_items, payments FROM " + role + "; REVOKE USAGE ON SCHEMA public FROM " + role + "; DROP ROLE " + role); err != nil {
			t.Error(err)
		}
	}()
	if _, err := db.Exec("GRANT USAGE ON SCHEMA public TO " + role + "; GRANT SELECT ON orders,order_items,payments TO " + role); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-c role="+role)
	q.Set("default_transaction_read_only", "on")
	u.RawQuery = q.Encode()
	readerDB, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer readerDB.Close()
	var current string
	if err := readerDB.Get(&current, "SELECT current_user"); err != nil || current != role {
		t.Fatal("read-only role was not applied")
	}
	if _, err := readerDB.Exec("UPDATE orders SET total_amount=0 WHERE false"); err == nil {
		t.Fatal("reader connection permitted a write")
	}
	client := newClient(t, repository.NewPostgresRepository(readerDB), time.Second, false)
	ctx := authorized(context.Background())
	request := &orderpb.GetOrderRequest{OrderId: orderID.String(), RequesterUserId: owner.String(), RequesterRole: "client"}
	out, err := client.GetOrder(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if out.TotalAmountMinor != 1005 || out.PaymentStatus != "success" || out.Status != orderpb.OrderStatus_CONFIRMED || out.CreatedAt != created.Format(time.RFC3339Nano) {
		t.Fatalf("incorrect persisted snapshot: %v", out)
	}
	if len(out.Items) != 1 || out.Items[0].ItemNameSnapshot != "Stored snapshot, not current menu" || out.Items[0].ItemPriceSnapshotMinor != 5 {
		t.Fatal("composite-key item isolation or exact money failed")
	}
	request.OrderId = foreignID.String()
	_, foreignErr := client.GetOrder(ctx, request)
	request.OrderId = uuid.NewString()
	_, missingErr := client.GetOrder(ctx, request)
	if status.Code(foreignErr) != codes.NotFound || status.Code(missingErr) != codes.NotFound || status.Convert(foreignErr).Message() != status.Convert(missingErr).Message() {
		t.Fatal("foreign/missing ownership boundary differs")
	}
	var persisted string
	if err := db.Get(&persisted, "SELECT total_amount::text FROM orders WHERE order_id=$1 AND created_at=$2", orderID, created); err != nil || persisted != "10.05" {
		t.Fatal("read changed stored order")
	}
}
