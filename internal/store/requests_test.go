package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/store"
	"github.com/bjschafer/print-dis/internal/store/storetest"
)

func TestRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	s := storetest.New(t)

	admin, err := s.EnsureDevUser(ctx, "Owner@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !admin.IsAdmin || *admin.Email != "owner@example.com" {
		t.Fatalf("dev user = %+v", admin)
	}
	if again, err := s.EnsureDevUser(ctx, "owner@example.com"); err != nil || again.ID != admin.ID {
		t.Fatalf("EnsureDevUser not idempotent: %v %v", again, err)
	}
	friend, err := s.CreatePlaceholder(ctx, "Friend", "friend@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePlaceholder(ctx, "Dup", "FRIEND@example.com"); err == nil {
		t.Fatal("duplicate email accepted")
	}

	p := &core.Printer{Name: "H2D", BedX: 325, BedY: 320, BedZ: 325, MultiMaterial: true, Active: true}
	if err := s.SavePrinter(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePrinter(ctx, &core.Printer{Name: "H2D", BedX: 1, BedY: 1, BedZ: 1}); !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("duplicate printer: %v", err)
	}

	due := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	mk := func(title string) *core.Request {
		r := &core.Request{RequesterID: friend.ID, CreatedByID: admin.ID, Title: title, Quantity: 2,
			Status: core.StatusRequested, Timing: core.TimingFirm, NeededBy: &due, Material: "PETG", SizeX: new(10), SizeY: new(20), SizeZ: new(30)}
		if err := s.CreateRequest(ctx, r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, b, c := mk("a"), mk("b"), mk("c")
	if !(a.Position < b.Position && b.Position < c.Position) {
		t.Fatalf("positions %v %v %v", a.Position, b.Position, c.Position)
	}

	got, err := s.GetRequest(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequesterName != "Friend" || got.Status != core.StatusRequested || *got.SizeZ != 30 || !got.NeededBy.Equal(due) {
		t.Fatalf("round trip: %+v", got)
	}

	got.PrinterID = &p.ID
	got.Printed = 1
	if err := s.UpdateRequest(ctx, got, admin.ID, "printer -> H2D"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus(ctx, a.ID, core.StatusPrinting, admin.ID); err != nil {
		t.Fatal(err)
	}
	// Drag c into printing above a.
	if err := s.Move(ctx, c.ID, core.StatusPrinting, []int64{c.ID, a.ID}, admin.ID); err != nil {
		t.Fatal(err)
	}
	printing, err := s.ListRequests(ctx, store.Filter{Statuses: []core.Status{core.StatusPrinting}})
	if err != nil {
		t.Fatal(err)
	}
	if len(printing) != 2 || printing[0].ID != c.ID || printing[1].ID != a.ID || printing[1].PrinterName != "H2D" {
		t.Fatalf("printing column = %+v", printing)
	}
	byPrinter, _ := s.ListRequests(ctx, store.Filter{PrinterID: p.ID})
	if len(byPrinter) != 1 {
		t.Fatalf("printer filter: %d", len(byPrinter))
	}
	sorted, _ := s.ListRequests(ctx, store.Filter{Sort: "title", Desc: true})
	if sorted[0].Title != "c" {
		t.Fatalf("sort: %v", sorted[0].Title)
	}

	if err := s.AddEvent(ctx, a.ID, friend.ID, core.EventComment, "thanks!"); err != nil {
		t.Fatal(err)
	}
	evs, err := s.ListEvents(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []core.EventKind
	for _, e := range evs {
		kinds = append(kinds, e.Kind)
	}
	if len(evs) != 3 || evs[1].Body != "requested -> printing" || evs[2].UserName != "Friend" {
		t.Fatalf("events %v %+v", kinds, evs)
	}
	if _, _, err := s.GetRequestImage(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("image: %v", err)
	}
	if _, err := s.GetRequest(ctx, 9999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
