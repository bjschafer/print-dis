package core

// These functions are the whole of the permission rules in design section 2.
// A nil user is never allowed anything.

func owns(u *User, r *Request) bool { return u != nil && r != nil && r.RequesterID == u.ID }

func isAdmin(u *User) bool { return u != nil && u.IsAdmin }

// requesterEditable statuses are the ones in which a requester may still edit.
func requesterEditable(s Status) bool { return s == StatusRequested || s == StatusNeedsInfo }

// CanView: admins see everything; requesters see their own.
func CanView(u *User, r *Request) bool { return isAdmin(u) || owns(u, r) }

// CanEdit: requesters may edit their own while it is requested or needs info.
func CanEdit(u *User, r *Request) bool {
	return isAdmin(u) || (owns(u, r) && requesterEditable(r.Status))
}

// CanComment: requesters may comment on their own at any time.
func CanComment(u *User, r *Request) bool { return isAdmin(u) || owns(u, r) }

// CanCancel: requesters may cancel their own before printing starts.
func CanCancel(u *User, r *Request) bool {
	if r == nil || r.Status == StatusCancelled {
		return false
	}
	return isAdmin(u) || (owns(u, r) && requesterEditable(r.Status))
}

// CanSetStatus: admins may make any change; requesters may only cancel.
func CanSetStatus(u *User, r *Request, to Status) bool {
	if r == nil || !to.Valid() || r.Status == to {
		return false
	}
	if to == StatusCancelled {
		return CanCancel(u, r)
	}
	return isAdmin(u)
}

// CanMove: dragging cards on the board is for admins.
func CanMove(u *User) bool { return isAdmin(u) }

// CanEditAdminFields: status, printer, progress, requester and position.
func CanEditAdminFields(u *User) bool { return isAdmin(u) }

// CanFileFor: only admins create requests on someone else's behalf.
func CanFileFor(u *User) bool { return isAdmin(u) }

// CanManagePrinters and CanManagePeople are admin pages.
func CanManagePrinters(u *User) bool { return isAdmin(u) }
func CanManagePeople(u *User) bool   { return isAdmin(u) }

// CanSeeAll decides whether lists are filtered to the user's own requests.
func CanSeeAll(u *User) bool { return isAdmin(u) }

// CanDownloadFile follows request visibility.
func CanDownloadFile(u *User, r *Request) bool { return CanView(u, r) }

// CanUploadFile follows edit permission.
func CanUploadFile(u *User, r *Request) bool { return CanEdit(u, r) }

// CanDeleteFile: admins always; requesters only their own uploads while they may edit.
func CanDeleteFile(u *User, r *Request, f *File) bool {
	if isAdmin(u) {
		return true
	}
	return f != nil && u != nil && f.UploadedByID == u.ID && f.RequestID == r.ID && CanEdit(u, r)
}
