package main

import (
	"context"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"ero/pkg/plugin"
)

type graphQLDoer interface {
	DoWithContext(ctx context.Context, query string, variables map[string]any, response any) error
}

func defaultGraphQLClient() (graphQLDoer, error) { return api.DefaultGraphQLClient() }

type graphQLClientFactory func() (graphQLDoer, error)

type githubPRSnapshot struct {
	Number        int
	URL           string
	Title         string
	State         string
	Body          string
	Author        string
	BaseRef       string
	HeadRef       string
	HeadRepoOwner string
	HeadRepoName  string
	HeadSHA       string
	UpdatedAt     *time.Time
	IssueComments []plugin.ProviderIssueComment
	Reviews       []plugin.ProviderReviewSummary
	Threads       []plugin.RemoteReviewThread
}

type ghPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type ghActor struct {
	Login string `json:"login"`
}

type ghPRListResponse struct {
	Repository struct {
		PullRequests struct {
			Nodes    []ghPRNode `json:"nodes"`
			PageInfo ghPageInfo `json:"pageInfo"`
		} `json:"pullRequests"`
	} `json:"repository"`
}

type ghPRSnapshotResponse struct {
	Repository struct {
		PullRequest ghPRNode `json:"pullRequest"`
	} `json:"repository"`
}

type ghPRNode struct {
	Number              int        `json:"number"`
	URL                 string     `json:"url"`
	Title               string     `json:"title"`
	State               string     `json:"state"`
	Body                string     `json:"body"`
	Author              *ghActor   `json:"author"`
	BaseRefName         string     `json:"baseRefName"`
	HeadRefName         string     `json:"headRefName"`
	HeadRefOid          string     `json:"headRefOid"`
	UpdatedAt           *time.Time `json:"updatedAt"`
	HeadRepositoryOwner *ghActor   `json:"headRepositoryOwner"`
	HeadRepository      *struct {
		Name string `json:"name"`
	} `json:"headRepository"`
	Comments struct {
		Nodes    []ghIssueComment `json:"nodes"`
		PageInfo ghPageInfo       `json:"pageInfo"`
	} `json:"comments"`
	Reviews struct {
		Nodes    []ghReview `json:"nodes"`
		PageInfo ghPageInfo `json:"pageInfo"`
	} `json:"reviews"`
	ReviewThreads struct {
		Nodes    []ghReviewThread `json:"nodes"`
		PageInfo ghPageInfo       `json:"pageInfo"`
	} `json:"reviewThreads"`
}

type ghIssueComment struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Body      string    `json:"body"`
	Author    *ghActor  `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type ghReview struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	Author      *ghActor  `json:"author"`
	SubmittedAt time.Time `json:"submittedAt"`
}
type ghReviewThread struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Side       string `json:"side"`
	StartLine  int    `json:"startLine"`
	StartSide  string `json:"startSide"`
	IsOutdated bool   `json:"isOutdated"`
	Comments   struct {
		Nodes    []ghReviewThreadComment `json:"nodes"`
		PageInfo ghPageInfo              `json:"pageInfo"`
	} `json:"comments"`
}
type ghReviewThreadComment struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Body      string    `json:"body"`
	Author    *ghActor  `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

const githubPRListQuery = `query EroPRList($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequests(first:50, after:$after, states:[OPEN]) {
      nodes {
        number
        url
        title
        state
        baseRefName
        headRefName
        headRefOid
        headRepositoryOwner { login }
        headRepository { name }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

const githubPRSnapshotQuery = `query EroPRSnapshot($owner:String!, $name:String!, $number:Int!, $commentsAfter:String, $reviewsAfter:String, $threadsAfter:String) {
  repository(owner:$owner, name:$name) {
    pullRequest(number:$number) {
      number
      url
      title
      state
      body
      updatedAt
      author { login }
      baseRefName
      headRefName
      headRefOid
      headRepositoryOwner { login }
      headRepository { name }
      comments(first:100, after:$commentsAfter) {
        nodes { id url body createdAt updatedAt author { login } }
        pageInfo { hasNextPage endCursor }
      }
      reviews(first:100, after:$reviewsAfter) {
        nodes { id url state body submittedAt author { login } }
        pageInfo { hasNextPage endCursor }
      }
      reviewThreads(first:100, after:$threadsAfter) {
        nodes {
          id
          path
          line
          side
          startLine
          startSide
          isOutdated
          comments(first:100) {
            nodes { id url body createdAt author { login } }
            pageInfo { hasNextPage endCursor }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

func (p githubProvider) graphQLClient() (graphQLDoer, error) {
	if p.newGraphQLClient != nil {
		return p.newGraphQLClient()
	}
	return defaultGraphQLClient()
}

func fetchGitHubSnapshot(ctx context.Context, client graphQLDoer, remote githubRemote, reviewCtx plugin.ReviewContext) (githubPRSnapshot, error) {
	candidates, err := fetchGitHubPRCandidates(ctx, client, remote)
	if err != nil {
		return githubPRSnapshot{}, err
	}
	match, err := matchGitHubPR(reviewCtx, candidates)
	if err != nil {
		return githubPRSnapshot{}, err
	}
	return fetchGitHubPRSnapshot(ctx, client, remote, match.Number)
}

func fetchGitHubPRCandidates(ctx context.Context, client graphQLDoer, remote githubRemote) ([]githubPRCandidate, error) {
	var out []githubPRCandidate
	after := ""
	for {
		vars := map[string]any{"owner": remote.Owner, "name": remote.Name, "after": cursorValue(after)}
		var resp ghPRListResponse
		if err := client.DoWithContext(ctx, githubPRListQuery, vars, &resp); err != nil {
			return nil, classifyGitHubRemoteError("fetch GitHub pull requests", err)
		}
		for _, n := range resp.Repository.PullRequests.Nodes {
			out = append(out, candidateFromNode(n))
		}
		pi := resp.Repository.PullRequests.PageInfo
		if !pi.HasNextPage {
			return out, nil
		}
		if pi.EndCursor == "" {
			return nil, plugin.NewError(plugin.ErrorRemoteValidationFailed, "GitHub pull request pagination missing end cursor")
		}
		after = pi.EndCursor
	}
}

func fetchGitHubPRSnapshot(ctx context.Context, client graphQLDoer, remote githubRemote, number int) (githubPRSnapshot, error) {
	var commentsAfter, reviewsAfter, threadsAfter string
	commentsDone, reviewsDone, threadsDone := false, false, false
	var accum githubPRSnapshot
	for {
		vars := map[string]any{"owner": remote.Owner, "name": remote.Name, "number": number, "commentsAfter": cursorValue(commentsAfter), "reviewsAfter": cursorValue(reviewsAfter), "threadsAfter": cursorValue(threadsAfter)}
		var resp ghPRSnapshotResponse
		if err := client.DoWithContext(ctx, githubPRSnapshotQuery, vars, &resp); err != nil {
			return githubPRSnapshot{}, classifyGitHubRemoteError("fetch GitHub pull request snapshot", err)
		}
		pr := resp.Repository.PullRequest
		page := mapGitHubPRMetadata(pr)
		if accum.Number == 0 {
			accum = page
		}
		if !commentsDone {
			accum.IssueComments = append(accum.IssueComments, mapGitHubIssueComments(pr.Comments.Nodes)...)
		}
		if !reviewsDone {
			accum.Reviews = append(accum.Reviews, mapGitHubReviews(pr.Reviews.Nodes)...)
		}
		if !threadsDone {
			for _, thread := range pr.ReviewThreads.Nodes {
				if thread.Comments.PageInfo.HasNextPage {
					return githubPRSnapshot{}, plugin.NewError(plugin.ErrorRemoteValidationFailed, "GitHub review thread comments pagination beyond first page is not supported")
				}
				accum.Threads = append(accum.Threads, mapGitHubThread(thread))
			}
		}
		if !commentsDone && pr.Comments.PageInfo.HasNextPage {
			if pr.Comments.PageInfo.EndCursor == "" {
				return githubPRSnapshot{}, plugin.NewError(plugin.ErrorRemoteValidationFailed, "GitHub comments pagination missing end cursor")
			}
			commentsAfter = pr.Comments.PageInfo.EndCursor
		} else {
			commentsDone = true
		}
		if !reviewsDone && pr.Reviews.PageInfo.HasNextPage {
			if pr.Reviews.PageInfo.EndCursor == "" {
				return githubPRSnapshot{}, plugin.NewError(plugin.ErrorRemoteValidationFailed, "GitHub reviews pagination missing end cursor")
			}
			reviewsAfter = pr.Reviews.PageInfo.EndCursor
		} else {
			reviewsDone = true
		}
		if !threadsDone && pr.ReviewThreads.PageInfo.HasNextPage {
			if pr.ReviewThreads.PageInfo.EndCursor == "" {
				return githubPRSnapshot{}, plugin.NewError(plugin.ErrorRemoteValidationFailed, "GitHub reviewThreads pagination missing end cursor")
			}
			threadsAfter = pr.ReviewThreads.PageInfo.EndCursor
		} else {
			threadsDone = true
		}
		if commentsDone && reviewsDone && threadsDone {
			return accum, nil
		}
	}
}

func cursorValue(cursor string) any {
	if cursor == "" {
		return nil
	}
	return cursor
}
