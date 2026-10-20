package mailpiece

// UserRequest is the body of PATCH /users/:id.json, sent flat: the controller wraps it under
// "user". A user takes only a name.
type UserRequest struct {
	Name string `json:"name"`
}
