package provider

// PRState represents the normalized state of a pull request across providers.
type PRState string

const (
	PROpen   PRState = "open"
	PRMerged PRState = "merged"
	PRClosed PRState = "closed"
)

// PullRequest holds provider-agnostic metadata for a pull request.
type PullRequest struct {
	Number int
	Title  string
	State  PRState
	Head   string // head branch name
	Base   string // target branch name
	URL    string
	Body   string
}
