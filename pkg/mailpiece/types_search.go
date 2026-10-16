package mailpiece

// SearchResults is searches/show.json.jbuilder: the posts and the subscribers a query
// matches, newest first.
type SearchResults struct {
	Posts       []Post       `json:"posts"`
	Subscribers []Subscriber `json:"subscribers"`
}
