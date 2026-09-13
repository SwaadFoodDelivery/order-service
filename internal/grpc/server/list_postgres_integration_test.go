package server_test

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"order-service/internal/services/order/repository"

	orderpb "github.com/SwaadFoodDelivery/proto/order"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This test never provisions or migrates a database. Root must serialize its use
// of the pre-provisioned disposable schema29 database with other integration work.
func TestGetUserOrdersPostgres(t *testing.T) {
	dsn := os.Getenv("ORDER_RPC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ORDER_RPC_TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || !regexp.MustCompile(`^/swaad_grpc_test_[a-z0-9_]+$`).MatchString(u.Path) {
		t.Fatal("requires dedicated swaad_grpc_test_* database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		t.Fatal("could not connect to dedicated integration database")
	}
	t.Cleanup(func() { db.Close() })
	var actualDB string
	if err := db.GetContext(ctx, &actualDB, "SELECT current_database()"); err != nil || actualDB != strings.TrimPrefix(u.Path, "/") {
		t.Fatal("resolved database differs from guarded name")
	}
	var version int
	if err := db.GetContext(ctx, &version, "SELECT version FROM schema_migrations WHERE NOT dirty"); err != nil || version != 29 {
		t.Fatal("requires clean backend schema29")
	}
	var postgis string
	if err := db.GetContext(ctx, &postgis, "SELECT postgis_version()"); err != nil {
		t.Fatal("requires PostGIS")
	}

	owner, foreign, restaurant := uuid.New(), uuid.New(), uuid.New()
	high, low := uuid.New(), uuid.New()
	if high.String() < low.String() {
		high, low = low, high
	}
	lastID, foreignID := uuid.New(), uuid.New()
	at := time.Now().UTC().Truncate(time.Second).Add(123456 * time.Microsecond)
	old := at.Add(-2 * time.Microsecond)
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, user := range []uuid.UUID{owner, foreign} {
		if _, err := tx.ExecContext(ctx, "INSERT INTO users(user_id,phone,name,role) VALUES($1,$2,'Fictional list RPC test','client')", user, user.String()[:14]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO restaurants(restaurant_id,owner_id,name,location,status)
		VALUES($1,$2,'Before rename',ST_SetSRID(ST_MakePoint(75.64,24.18),4326),'inactive')`, restaurant, owner); err != nil {
		t.Fatal(err)
	}
	insertOrder := `INSERT INTO orders(order_id,created_at,user_id,restaurant_id,status,subtotal,taxes,delivery_fee,total_amount,payment_method)
		VALUES($1,$2,$3,$4,'confirmed',$5,0,0,$5,'cash_on_delivery')`
	for _, row := range []struct {
		id, user uuid.UUID
		created  time.Time
		total    string
	}{
		{high, owner, at, "10.05"}, {low, owner, at, "0.05"}, {high, owner, old, "1.05"},
		{lastID, owner, at.Add(-time.Second), "20.00"}, {foreignID, foreign, at.Add(time.Second), "99.99"},
	} {
		if _, err := tx.ExecContext(ctx, insertOrder, row.id, row.created, row.user, restaurant, row.total); err != nil {
			t.Fatal(err)
		}
	}
	for _, delivery := range []struct {
		created time.Time
		status  string
	}{{at, "picked_up"}, {old, "delivered"}} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO deliveries(order_id,order_created_at,partner_id,status) VALUES($1,$2,$3,$4)`, high, delivery.created, owner, delivery.status); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// Rename the private inactive restaurant after placing fixtures: list returns
	// the current name without filtering inactive historical restaurants.
	if _, err := db.ExecContext(ctx, "UPDATE restaurants SET name='Current inactive restaurant' WHERE restaurant_id=$1", restaurant); err != nil {
		t.Fatal(err)
	}

	reader := listReader(t, db, u)
	client := newClient(t, repository.NewPostgresRepository(reader), time.Second, false)
	rpcCtx := authorized(ctx)
	request := &orderpb.GetUserOrdersRequest{UserId: owner.String(), RequesterRole: "client", Limit: 2}
	page1, err := client.GetUserOrders(rpcCtx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Orders) != 2 || page1.NextCursor == "" || page1.Total != 0 {
		t.Fatalf("unexpected first page: %v", page1)
	}
	if page1.Orders[0].OrderId != high.String() || page1.Orders[1].OrderId != low.String() {
		t.Fatal("UUID descending tie-break or ownership failed")
	}
	for _, row := range page1.Orders {
		if row.CreatedAt != at.Format(time.RFC3339Nano) || row.RestaurantName != "Current inactive restaurant" || row.RestaurantId != restaurant.String() || row.Currency != "INR" ||
			row.PaymentMethod != "cash_on_delivery" || row.PaymentStatus != "" || len(row.Items) != 0 {
			t.Fatal("summary mapping, microseconds, or inactive restaurant failed")
		}
	}
	if page1.Orders[0].TotalAmountMinor != 1005 || page1.Orders[0].TotalAmount != 10.05 || page1.Orders[1].TotalAmountMinor != 5 || page1.Orders[1].TotalAmount != 0.05 {
		t.Fatal("strict exact money mapping failed")
	}
	if page1.Orders[0].DeliveryStatus != "picked_up" || page1.Orders[1].DeliveryStatus != "" {
		t.Fatal("composite delivery join or missing delivery preservation failed")
	}

	// Cursors are positions, not snapshots. A new top row is outside the cursor;
	// a backfilled row below the position is visible on the next page.
	newTop, backfill := uuid.New(), uuid.New()
	for _, row := range []struct {
		id      uuid.UUID
		created time.Time
	}{{newTop, at.Add(time.Second)}, {backfill, at.Add(-time.Microsecond)}} {
		if _, err := db.ExecContext(ctx, insertOrder, row.id, row.created, owner, restaurant, "0.05"); err != nil {
			t.Fatal(err)
		}
	}
	request.Cursor = page1.NextCursor
	page2, err := client.GetUserOrders(rpcCtx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Orders) != 2 || page2.NextCursor == "" || page2.Orders[0].OrderId != backfill.String() || page2.Orders[1].OrderId != high.String() ||
		page2.Orders[1].CreatedAt != old.Format(time.RFC3339Nano) || page2.Orders[1].DeliveryStatus != "delivered" || page2.Orders[1].TotalAmountMinor != 105 {
		t.Fatal("interpage inserts, precise cursor, or composite delivery isolation failed")
	}
	request.Cursor = page2.NextCursor
	page3, err := client.GetUserOrders(rpcCtx, request)
	if err != nil || len(page3.Orders) != 1 || page3.NextCursor != "" || page3.Orders[0].OrderId != lastID.String() {
		t.Fatalf("incorrect terminal page: %v, %v", page3, err)
	}

	request.Cursor, request.Limit = "", 6
	all, err := client.GetUserOrders(rpcCtx, request)
	if err != nil || len(all.Orders) != 6 || all.NextCursor != "" || all.Orders[0].OrderId != newTop.String() || all.Total != 0 {
		t.Fatal("exact page boundary or new first-page view failed")
	}
	request.Limit = 50
	maxPage, err := client.GetUserOrders(rpcCtx, request)
	if err != nil || len(maxPage.Orders) != 6 || maxPage.NextCursor != "" {
		t.Fatal("maximum limit failed")
	}
	request.UserId = uuid.NewString()
	empty, err := client.GetUserOrders(rpcCtx, request)
	if err != nil || len(empty.Orders) != 0 || empty.NextCursor != "" || empty.Total != 0 {
		t.Fatal("empty owner page failed")
	}
	request.UserId, request.Cursor = foreign.String(), page1.NextCursor
	if _, err := client.GetUserOrders(rpcCtx, request); status.Code(err) != codes.InvalidArgument {
		t.Fatal("accepted cursor for another requester")
	}
	request.Cursor = ""
	foreignPage, err := client.GetUserOrders(rpcCtx, request)
	if err != nil || len(foreignPage.Orders) != 1 || foreignPage.Orders[0].OrderId != foreignID.String() {
		t.Fatal("SQL ownership filter failed")
	}
	var persisted string
	if err := db.GetContext(ctx, &persisted, "SELECT total_amount::text FROM orders WHERE order_id=$1 AND created_at=$2", high, at); err != nil || persisted != "10.05" {
		t.Fatal("list changed order data")
	}
	t.Log("verified tied times, microsecond cursors, interpage inserts, owner filtering, exact money, and composite deliveries with a SELECT-only role")
}

func listReader(t *testing.T, db *sqlx.DB, endpoint *url.URL) *sqlx.DB {
	t.Helper()
	role := "swaad_rpc_list_reader_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.Exec("CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE"); err != nil {
		t.Fatal(err)
	}
	tables := "orders,order_items,payments,restaurants,deliveries"
	t.Cleanup(func() {
		if _, err := db.Exec("REVOKE SELECT ON " + tables + " FROM " + role + "; REVOKE USAGE ON SCHEMA public FROM " + role + "; DROP ROLE " + role); err != nil {
			t.Error("temporary list reader role cleanup failed")
		}
	})
	if _, err := db.Exec("GRANT USAGE ON SCHEMA public TO " + role + "; GRANT SELECT ON " + tables + " TO " + role); err != nil {
		t.Fatal(err)
	}
	u := *endpoint
	q := u.Query()
	q.Set("options", "-c role="+role)
	q.Set("default_transaction_read_only", "on")
	u.RawQuery = q.Encode()
	reader, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatal("could not connect as temporary list reader")
	}
	t.Cleanup(func() { reader.Close() })
	var current string
	if err := reader.Get(&current, "SELECT current_user"); err != nil || current != role {
		t.Fatal("reader role was not applied")
	}
	for _, table := range []string{"orders", "order_items", "payments", "restaurants", "deliveries"} {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
			var allowed bool
			if err := reader.Get(&allowed, "SELECT has_table_privilege(current_user,$1,$2)", table, privilege); err != nil {
				t.Fatal(err)
			}
			if allowed != (privilege == "SELECT") {
				t.Fatalf("reader privilege %s on %s = %v", privilege, table, allowed)
			}
		}
	}
	return reader
}
