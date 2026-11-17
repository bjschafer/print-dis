package core

import (
	"slices"
	"testing"
)

func TestEmailsRequester(t *testing.T) {
	addr := "alice@example.com"
	empty := ""
	owner := &User{ID: 1, IsAdmin: true}
	alice := &User{ID: 2, Email: &addr}
	noEmail := &User{ID: 3}
	blank := &User{ID: 4, Email: &empty}
	tests := []struct {
		name      string
		actor     *User
		requester *User
		from, to  Status
		want      bool
	}{
		{"ready", owner, alice, StatusPrinting, StatusReady, true},
		{"needs info", owner, alice, StatusRequested, StatusNeedsInfo, true},
		{"straight to ready", owner, alice, StatusRequested, StatusReady, true},
		{"printing", owner, alice, StatusRequested, StatusPrinting, false},
		{"delivered", owner, alice, StatusReady, StatusDelivered, false},
		{"cancelled", owner, alice, StatusRequested, StatusCancelled, false},
		{"unchanged", owner, alice, StatusReady, StatusReady, false},
		{"own change", alice, alice, StatusPrinting, StatusReady, false},
		{"no email", owner, noEmail, StatusPrinting, StatusReady, false},
		{"blank email", owner, blank, StatusPrinting, StatusReady, false},
		{"nil actor", nil, alice, StatusPrinting, StatusReady, false},
		{"nil requester", owner, nil, StatusPrinting, StatusReady, false},
	}
	for _, tt := range tests {
		if got := EmailsRequester(tt.actor, tt.requester, tt.from, tt.to); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestNewRequestRecipients(t *testing.T) {
	a, b, c := "a@example.com", "b@example.com", "c@example.com"
	users := []User{
		{ID: 1, IsAdmin: true, Email: &a},
		{ID: 2, IsAdmin: true, Email: &b},
		{ID: 3, IsAdmin: true},
		{ID: 4, Email: &c},
	}
	ids := func(us []User) []int64 {
		var out []int64
		for _, u := range us {
			out = append(out, u.ID)
		}
		return out
	}
	if got := ids(NewRequestRecipients(&users[3], users)); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("friend files: got %v", got)
	}
	if got := ids(NewRequestRecipients(&users[0], users)); !slices.Equal(got, []int64{2}) {
		t.Errorf("admin files: got %v", got)
	}
	if got := NewRequestRecipients(&User{ID: 9}, nil); got != nil {
		t.Errorf("no users: got %v", got)
	}
}
