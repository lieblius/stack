package provider

import (
	"fmt"
	"time"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v2"
)

// Forgejo implements Host using the Forgejo REST API via the Go SDK.
type Forgejo struct {
	client *forgejo.Client
	owner  string
	repo   string
}

// NewForgejo creates a Forgejo provider for the given instance.
// The token is used for API authentication.
func NewForgejo(instanceURL, token, owner, repo string) (*Forgejo, error) {
	client, err := forgejo.NewClient(instanceURL, forgejo.SetToken(token))
	if err != nil {
		return nil, fmt.Errorf("creating Forgejo client for %s: %w", instanceURL, err)
	}
	return &Forgejo{client: client, owner: owner, repo: repo}, nil
}

func (f *Forgejo) toPullRequest(pr *forgejo.PullRequest) *PullRequest {
	state := PROpen
	if pr.HasMerged {
		state = PRMerged
	} else if pr.State == forgejo.StateClosed {
		state = PRClosed
	}

	head := ""
	if pr.Head != nil {
		head = pr.Head.Ref
	}
	base := ""
	if pr.Base != nil {
		base = pr.Base.Ref
	}

	return &PullRequest{
		Number: int(pr.Index),
		Title:  pr.Title,
		State:  state,
		Head:   head,
		Base:   base,
		URL:    pr.HTMLURL,
		Body:   pr.Body,
	}
}

func (f *Forgejo) PRForBranch(branch string) (*PullRequest, error) {
	page := 1
	for {
		prs, _, err := f.client.ListRepoPullRequests(f.owner, f.repo, forgejo.ListPullRequestsOptions{
			ListOptions: forgejo.ListOptions{Page: page, PageSize: 50},
			State:       forgejo.StateAll,
		})
		if err != nil {
			return nil, fmt.Errorf("listing PRs: %w", err)
		}
		if len(prs) == 0 {
			return nil, nil
		}
		for _, pr := range prs {
			if pr.Head != nil && pr.Head.Ref == branch {
				return f.toPullRequest(pr), nil
			}
		}
		page++
	}
}

func (f *Forgejo) CreatePR(head, base, title, body string) (*PullRequest, error) {
	pr, _, err := f.client.CreatePullRequest(f.owner, f.repo, forgejo.CreatePullRequestOption{
		Head:  head,
		Base:  base,
		Title: title,
		Body:  body,
	})
	if err != nil {
		return nil, fmt.Errorf("creating PR: %w", err)
	}
	return f.toPullRequest(pr), nil
}

func (f *Forgejo) EditPRBase(number int, newBase string) error {
	_, _, err := f.client.EditPullRequest(f.owner, f.repo, int64(number), forgejo.EditPullRequestOption{
		Base: newBase,
	})
	if err != nil {
		return fmt.Errorf("editing PR #%d base: %w", number, err)
	}
	return nil
}

func (f *Forgejo) EditPRBody(number int, body string) error {
	_, _, err := f.client.EditPullRequest(f.owner, f.repo, int64(number), forgejo.EditPullRequestOption{
		Body: body,
	})
	if err != nil {
		return fmt.Errorf("editing PR #%d body: %w", number, err)
	}
	return nil
}

func (f *Forgejo) MergePR(number int, strategy MergeStrategy) error {
	var style forgejo.MergeStyle
	switch strategy {
	case MergeSquash:
		style = forgejo.MergeStyleSquash
	case MergeMerge:
		style = forgejo.MergeStyleMerge
	case MergeRebase:
		style = forgejo.MergeStyleRebase
	default:
		style = forgejo.MergeStyleSquash
	}

	// Forgejo may not be ready to merge immediately after a rebase.
	// Retry briefly to allow the merge state to settle.
	for attempt := range 3 {
		ok, _, err := f.client.MergePullRequest(f.owner, f.repo, int64(number), forgejo.MergePullRequestOption{
			Style: style,
		})
		if err != nil {
			return fmt.Errorf("merging PR #%d: %w", number, err)
		}
		if ok {
			return nil
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	return fmt.Errorf("merging PR #%d: merge was not successful after retries", number)
}
