package core

// EmailsRequester reports whether the requester is emailed when actor moves
// one of their requests from `from` to `to`: on ready ("come and get it")
// and needs info ("I have a question"). Nobody is emailed about their own
// change, and someone without an email address cannot be.
func EmailsRequester(actor, requester *User, from, to Status) bool {
	if actor == nil || requester == nil || from == to || !hasEmail(requester) || actor.ID == requester.ID {
		return false
	}
	return to == StatusReady || to == StatusNeedsInfo
}

// NewRequestRecipients are the people emailed when filer files a request:
// every admin with an email address except the filer.
func NewRequestRecipients(filer *User, users []User) []User {
	var out []User
	for _, u := range users {
		if u.IsAdmin && hasEmail(&u) && (filer == nil || u.ID != filer.ID) {
			out = append(out, u)
		}
	}
	return out
}

func hasEmail(u *User) bool { return u.Email != nil && *u.Email != "" }
