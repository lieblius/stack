package provider

type PRState string

const (
	PROpen   PRState = "open"
	PRMerged PRState = "merged"
	PRClosed PRState = "closed"
)

type MergeStrategy string

const (
	MergeSquash MergeStrategy = "squash"
	MergeMerge  MergeStrategy = "merge"
	MergeRebase MergeStrategy = "rebase"
)

type PullRequest struct {
	Number int
	Title  string
	State  PRState
	Head   string // head branch name
	Base   string // target branch name
	URL    string
	Body   string
}

type Host interface {
	PRForBranch(branch string) (*PullRequest, error)
	CreatePR(head, base, title, body string) (*PullRequest, error)
	EditPRBase(number int, newBase string) error
	EditPRBody(number int, body string) error
	MergePR(number int, strategy MergeStrategy) error
}
