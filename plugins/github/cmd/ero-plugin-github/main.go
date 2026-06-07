package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	gh "github.com/cli/go-gh/v2"

	"ero/pkg/plugin"
)

const providerID = "github"

type githubProvider struct {
	getenv           func(string) string
	execGH           func(context.Context, ...string) (string, string, error)
	newGraphQLClient graphQLClientFactory
}

type ghPR struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

type ghReviewResponse struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
}

func main() {
	provider := githubProvider{getenv: os.Getenv}
	if err := plugin.ServeReviewProvider(context.Background(), provider, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p githubProvider) Initialize(_ context.Context, req plugin.InitializeRequest) (plugin.InitializeResult, error) {
	if req.Protocol != plugin.ProtocolVersion {
		return plugin.InitializeResult{}, plugin.NewErrorf(plugin.ErrorInvalidRequest, "unsupported protocol %q", req.Protocol)
	}
	if req.ContributionID != "" && req.ContributionID != providerID {
		return plugin.InitializeResult{}, plugin.NewErrorf(plugin.ErrorInvalidRequest, "unsupported contribution %q", req.ContributionID)
	}
	return plugin.InitializeResult{
		Protocol: plugin.ProtocolVersion,
		Provider: plugin.ReviewProviderInfo{
			ID:    providerID,
			Label: "GitHub",
			Name:  "ero-plugin-github",
			Capabilities: plugin.ReviewProviderCapabilities{
				LoadRemoteComments: true,
				LoadRemoteSnapshot: true,
				PublishReview:      true,
				Decisions: []plugin.ReviewDecision{
					plugin.ReviewDecisionComment,
					plugin.ReviewDecisionRequestChanges,
					plugin.ReviewDecisionApprove,
				},
				IdempotentPublish: false,
			},
		},
	}, nil
}

func (p githubProvider) DetectContext(ctx context.Context, req plugin.DetectContextRequest) (plugin.DetectContextResult, error) {
	if !isBranchReviewMode(req.Context) {
		return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: false, Reason: "GitHub PR sync is available in branch mode only"}}, nil
	}
	remotes := githubRemotes(req.Context.Repository.Remotes)
	if len(remotes) == 0 {
		return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: false, Reason: "no GitHub remote detected"}}, nil
	}
	client, err := p.graphQLClient()
	if err != nil {
		return plugin.DetectContextResult{}, plugin.NewErrorf(plugin.ErrorAuthRequired, "create GitHub GraphQL client: %v", err)
	}
	_, match, err := matchGitHubPRAcrossRemotes(ctx, client, remotes, req.Context)
	if err != nil {
		if pe := plugin.AsError(err); pe != nil && pe.Code == plugin.ErrorNotApplicable {
			return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: false, Reason: err.Error()}}, nil
		}
		return plugin.DetectContextResult{}, err
	}
	return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: true, Reason: "matched GitHub pull request " + githubPRSummary(match)}}, nil
}

func (p githubProvider) LoadRemoteSnapshot(ctx context.Context, req plugin.LoadRemoteSnapshotRequest) (plugin.LoadRemoteSnapshotResult, error) {
	if !isBranchReviewMode(req.Context) {
		return plugin.LoadRemoteSnapshotResult{}, plugin.NewError(plugin.ErrorNotApplicable, "GitHub PR sync is available in branch mode only")
	}
	remotes := githubRemotes(req.Context.Repository.Remotes)
	if len(remotes) == 0 {
		return plugin.LoadRemoteSnapshotResult{}, plugin.NewError(plugin.ErrorNotApplicable, "no GitHub remote detected")
	}
	client, err := p.graphQLClient()
	if err != nil {
		return plugin.LoadRemoteSnapshotResult{}, plugin.NewErrorf(plugin.ErrorAuthRequired, "create GitHub GraphQL client: %v", err)
	}
	remote, match, err := matchGitHubPRAcrossRemotes(ctx, client, remotes, req.Context)
	if err != nil {
		return plugin.LoadRemoteSnapshotResult{}, err
	}
	snapshot, err := fetchGitHubPRSnapshot(ctx, client, remote, match.Number)
	if err != nil {
		return plugin.LoadRemoteSnapshotResult{}, err
	}
	return snapshotResultFromGitHub(snapshot), nil
}

func (p githubProvider) LoadRemoteThreads(ctx context.Context, req plugin.LoadRemoteThreadsRequest) (plugin.LoadRemoteThreadsResult, error) {
	snapshotReq := plugin.LoadRemoteSnapshotRequest(req)
	snapshot, err := p.LoadRemoteSnapshot(ctx, snapshotReq)
	if err != nil {
		return plugin.LoadRemoteThreadsResult{}, err
	}
	return plugin.LoadRemoteThreadsResult{Threads: snapshot.Threads}, nil
}

func (p githubProvider) PublishReview(ctx context.Context, req plugin.PublishReviewParams) (plugin.PublishReviewResultData, error) {
	if p.execGH == nil {
		p.execGH = execGH
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	pr, err := p.currentPullRequest(ctx, req.Payload.Context)
	if err != nil {
		return plugin.PublishReviewResultData{}, err
	}
	args, err := buildReviewArgs(pr.Number, req.Payload)
	if err != nil {
		return plugin.PublishReviewResultData{}, err
	}
	stdout, stderr, err := p.execGH(ctx, args...)
	if err != nil {
		message := strings.TrimSpace(stderr)
		if message == "" {
			message = err.Error()
		}
		return plugin.PublishReviewResultData{}, classifyGitHubRemoteMessage("publish GitHub review", message)
	}
	var response ghReviewResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		return plugin.PublishReviewResultData{}, plugin.NewErrorf(plugin.ErrorRemoteValidationFailed, "parse GitHub review publish response: %v", err)
	}
	if response.ID == 0 {
		return plugin.PublishReviewResultData{}, plugin.NewError(plugin.ErrorRemoteValidationFailed, "GitHub review publish response did not include a review id")
	}
	return plugin.PublishReviewResultData{Result: plugin.ReviewPublishResult{
		ProviderID:       providerID,
		ExternalReviewID: githubReviewID(response, pr),
		ExternalURL:      firstNonEmpty(response.HTMLURL, pr.URL),
		PublishedRefs:    githubPublishedRefs(req.Payload.Draft.Comments),
	}}, nil
}

func (p githubProvider) currentPullRequest(ctx context.Context, reviewCtx plugin.ReviewContext) (ghPR, error) {
	if remotes := githubRemotes(reviewCtx.Repository.Remotes); len(remotes) > 0 {
		if client, err := p.graphQLClient(); err == nil {
			_, match, err := matchGitHubPRAcrossRemotes(ctx, client, remotes, reviewCtx)
			if err == nil {
				return ghPR{Number: match.Number, URL: match.URL}, nil
			}
			if pe := plugin.AsError(err); pe == nil || pe.Code != plugin.ErrorNotApplicable {
				return ghPR{}, err
			}
		}
	}
	if p.execGH == nil {
		p.execGH = execGH
	}
	stdout, stderr, err := p.execGH(ctx, ghPRViewArgs(reviewCtx)...)
	if err != nil {
		message := strings.TrimSpace(stderr)
		if message == "" {
			message = err.Error()
		}
		if strings.Contains(strings.ToLower(message), "no pull request") || strings.Contains(strings.ToLower(message), "no pull requests") {
			return ghPR{}, plugin.NewErrorf(plugin.ErrorNotApplicable, "no pull request found for review context: %s", message)
		}
		return ghPR{}, classifyGitHubRemoteMessage("GitHub CLI PR lookup failed", message)
	}
	var pr ghPR
	if err := json.Unmarshal([]byte(stdout), &pr); err != nil {
		return ghPR{}, plugin.NewErrorf(plugin.ErrorRemoteValidationFailed, "parse GitHub PR lookup response: %v", err)
	}
	if pr.Number == 0 {
		return ghPR{}, plugin.NewError(plugin.ErrorNotApplicable, "no pull request found for current branch")
	}
	return pr, nil
}

func ghPRViewArgs(reviewCtx plugin.ReviewContext) []string {
	args := []string{"pr", "view", "--json", "number,url"}
	branch := publishPRLookupBranch(reviewCtx)
	if branch != "" {
		args = append(args, branch)
	}
	return args
}

func isBranchReviewMode(reviewCtx plugin.ReviewContext) bool {
	mode := reviewCtx.Target.Mode
	return mode == "" || strings.EqualFold(mode, "branch")
}

func publishPRLookupBranch(reviewCtx plugin.ReviewContext) string {
	headRef := strings.TrimSpace(reviewCtx.Target.HeadRef)
	if headRef == "" && strings.EqualFold(reviewCtx.Target.Mode, "branch") {
		headRef = strings.TrimSpace(reviewCtx.Repository.CurrentBranch)
	}
	if headRef == "" {
		return ""
	}
	return normalizeRef(headRef)
}

func classifyGitHubRemoteError(action string, err error) error {
	if pe := plugin.AsError(err); pe != nil {
		return pe
	}
	return classifyGitHubRemoteMessage(action, err.Error())
}

func classifyGitHubRemoteMessage(action, message string) error {
	lower := strings.ToLower(message)
	code := plugin.ErrorNetwork
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "secondary rate") || strings.Contains(lower, "api rate limit exceeded") {
		code = plugin.ErrorRemoteRateLimited
	} else if strings.Contains(lower, "auth") || strings.Contains(lower, "authentication") || strings.Contains(lower, "credential") || strings.Contains(lower, "401") || strings.Contains(lower, "403") {
		code = plugin.ErrorAuthRequired
	}
	return plugin.NewErrorf(code, "%s: %s", action, message)
}

func buildReviewArgs(prNumber int, payload plugin.ReviewPublishPayload) ([]string, error) {
	args := []string{"api", "-X", "POST", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/reviews", prNumber)}
	if payload.Context.Repository.HeadSHA != "" {
		args = append(args, "-f", "commit_id="+payload.Context.Repository.HeadSHA)
	}
	event := githubReviewEvent(payload.Draft.Decision)
	args = append(args, "-f", "event="+event)
	body := strings.TrimSpace(payload.Draft.Summary)
	if body == "" && len(payload.Draft.Comments) == 0 {
		body = "Ero review published."
	}
	if body != "" {
		args = append(args, "-f", "body="+body)
	}
	for _, comment := range payload.Draft.Comments {
		commentArgs, err := githubCommentArgs(comment)
		if err != nil {
			return nil, err
		}
		args = append(args, commentArgs...)
	}
	return args, nil
}

func githubCommentArgs(comment plugin.ReviewComment) ([]string, error) {
	body := strings.TrimSpace(comment.Body)
	if body == "" {
		return nil, plugin.NewError(plugin.ErrorInvalidRequest, "GitHub review comment body is empty")
	}
	line, side, ok := githubLineAndSide(comment.Range.End)
	if !ok {
		return nil, plugin.NewErrorf(plugin.ErrorInvalidRequest, "GitHub review comment %s has no mappable end line", comment.ID)
	}
	args := []string{
		"-f", "comments[][path]=" + comment.FilePath,
		"-f", "comments[][body]=" + body,
		"-F", "comments[][line]=" + strconv.Itoa(line),
		"-f", "comments[][side]=" + side,
	}
	if startLine, startSide, ok := githubLineAndSide(comment.Range.Start); ok && startLine != line {
		args = append(args, "-F", "comments[][start_line]="+strconv.Itoa(startLine), "-f", "comments[][start_side]="+startSide)
	}
	return args, nil
}

func githubLineAndSide(ref plugin.ReviewLineRef) (int, string, bool) {
	if ref.NewLineNumber > 0 {
		return ref.NewLineNumber, "RIGHT", true
	}
	if ref.OldLineNumber > 0 {
		return ref.OldLineNumber, "LEFT", true
	}
	return 0, "", false
}

func githubReviewEvent(decision plugin.ReviewDecision) string {
	switch decision {
	case plugin.ReviewDecisionApprove:
		return "APPROVE"
	case plugin.ReviewDecisionRequestChanges:
		return "REQUEST_CHANGES"
	default:
		return "COMMENT"
	}
}

func githubPublishedRefs(comments []plugin.ReviewComment) []plugin.PublishedReviewCommentRef {
	refs := make([]plugin.PublishedReviewCommentRef, len(comments))
	for i, comment := range comments {
		refs[i] = plugin.PublishedReviewCommentRef{LocalCommentID: comment.ID}
	}
	return refs
}

func githubReviewID(response ghReviewResponse, pr ghPR) string {
	if response.ID > 0 {
		return strconv.FormatInt(response.ID, 10)
	}
	return fmt.Sprintf("pr-%d", pr.Number)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func execGH(ctx context.Context, args ...string) (string, string, error) {
	stdout, stderr, err := gh.ExecContext(ctx, args...)
	return stdout.String(), stderr.String(), err
}
