// Package core holds the domain types and the rules about them. It does no IO.
package core

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type Status string

const (
	StatusRequested Status = "requested"
	StatusNeedsInfo Status = "needs_info"
	StatusPrinting  Status = "printing"
	StatusReady     Status = "ready"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
)

// Statuses lists every status in lifecycle order.
var Statuses = []Status{StatusRequested, StatusNeedsInfo, StatusPrinting, StatusReady, StatusDelivered, StatusCancelled}

// BoardStatuses are the columns shown on the board.
var BoardStatuses = []Status{StatusRequested, StatusNeedsInfo, StatusPrinting, StatusReady}

func (s Status) Valid() bool { return slices.Contains(Statuses, s) }

// Terminal statuses are hidden from the board.
func (s Status) Terminal() bool { return s == StatusDelivered || s == StatusCancelled }

// Label is the human name, matching the old sheet.
func (s Status) Label() string {
	switch s {
	case StatusRequested:
		return "Requested"
	case StatusNeedsInfo:
		return "Needs info"
	case StatusPrinting:
		return "Printing"
	case StatusReady:
		return "Ready for pickup"
	case StatusDelivered:
		return "Delivered"
	case StatusCancelled:
		return "Cancelled"
	}
	return string(s)
}

type Timing string

const (
	TimingFirm     Timing = "firm"
	TimingFlexible Timing = "flexible"
	TimingWhenever Timing = "whenever"
)

var Timings = []Timing{TimingFirm, TimingFlexible, TimingWhenever}

func (t Timing) Valid() bool { return t == TimingFirm || t == TimingFlexible || t == TimingWhenever }

func (t Timing) Label() string {
	switch t {
	case TimingFirm:
		return "Firm"
	case TimingFlexible:
		return "Flexible"
	}
	return "Whenever"
}

type User struct {
	ID        int64
	OIDCSub   *string // nil for placeholders
	Email     *string
	Name      string
	IsAdmin   bool
	CreatedAt time.Time
}

// Placeholder reports whether the user was created by an admin and has never signed in.
func (u *User) Placeholder() bool { return u.OIDCSub == nil }

type Printer struct {
	ID            int64
	Name          string
	BedX          int
	BedY          int
	BedZ          int
	MultiMaterial bool
	Enclosed      bool
	Notes         string
	Active        bool
}

type Request struct {
	ID          int64
	RequesterID int64
	CreatedByID int64
	Title       string
	Quantity    int
	Printed     int
	Status      Status
	NeededBy    *time.Time // a date; time of day is meaningless
	Timing      Timing
	Notes       string

	SourceURL       *string
	SourceTitle     *string
	SourceImage     []byte
	SourceImageType *string

	Material           string
	SpoolmanFilamentID *int
	FilamentLabel      string
	FilamentColorHex   *string
	ColorNote          string
	NeedsMultiMaterial bool
	NeedsEnclosure     bool
	SizeX, SizeY       *int
	SizeZ              *int

	PrinterID *int64
	Position  float64
	CreatedAt time.Time
	UpdatedAt time.Time

	// Joined for display; not columns of requests.
	RequesterName string
	PrinterName   string
}

// HasSize reports whether all three dimensions are known.
func (r *Request) HasSize() bool { return r.SizeX != nil && r.SizeY != nil && r.SizeZ != nil }

// Urgent reports whether a firm deadline is overdue or within three days of today.
func (r *Request) Urgent(today time.Time) bool {
	if r.Timing != TimingFirm || r.NeededBy == nil || r.Status.Terminal() {
		return false
	}
	y, m, d := today.Date()
	limit := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 3)
	ny, nm, nd := r.NeededBy.Date()
	return !time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC).After(limit)
}

type EventKind string

const (
	EventComment EventKind = "comment"
	EventStatus  EventKind = "status"
	EventEdit    EventKind = "edit"
	EventFile    EventKind = "file"
)

type Event struct {
	ID        int64
	RequestID int64
	UserID    int64
	UserName  string
	Kind      EventKind
	Body      string
	CreatedAt time.Time
}

type File struct {
	ID           int64
	RequestID    int64
	UploadedByID int64
	UploaderName string
	ObjectKey    string
	Filename     string
	ContentType  string
	SizeBytes    int64
	SHA256       string
	CreatedAt    time.Time
}

// StatusChangeBody is the event text for a status change.
func StatusChangeBody(from, to Status) string { return fmt.Sprintf("%s -> %s", from, to) }

// enclosureMaterials are the materials that pre-tick "needs enclosure".
var enclosureMaterials = map[string]bool{"ABS": true, "ASA": true, "PC": true, "PA": true}

// MaterialWantsEnclosure reports whether the form should pre-tick "needs enclosure".
// It matches the leading material family, so "ABS-GF" and "PA12-CF" count.
func MaterialWantsEnclosure(material string) bool {
	m := strings.ToUpper(strings.TrimSpace(material))
	if enclosureMaterials[m] {
		return true
	}
	for k := range enclosureMaterials {
		if strings.HasPrefix(m, k) && len(m) > len(k) {
			next := m[len(k)]
			if next == '-' || next == '+' || next == ' ' || (next >= '0' && next <= '9') {
				return true
			}
		}
	}
	return false
}
