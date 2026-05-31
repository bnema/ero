package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ero/pkg/plugin"
)

const (
	providerID      = "pi-coding-agent"
	bridgeRelDir    = "ero-pi-coding-agent-bridge"
	bridgeStateFile = "sessions.json"
)

type piProvider struct {
	getenv    func(string) string
	statePath string
}

type bridgeState struct {
	Sessions []bridgeSession `json:"sessions"`
}

type bridgeSession struct {
	SessionID     string `json:"session_id"`
	SessionFile   string `json:"session_file,omitempty"`
	Cwd           string `json:"cwd"`
	WorktreeRoot  string `json:"worktree_root,omitempty"`
	CurrentBranch string `json:"current_branch,omitempty"`
	HeadSHA       string `json:"head_sha,omitempty"`
	SocketPath    string `json:"socket_path"`
	Token         string `json:"token"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type bridgeReviewMessage struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type bridgeResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func main() {
	provider := piProvider{getenv: os.Getenv}
	if err := plugin.ServeReviewProvider(context.Background(), provider, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p piProvider) Initialize(_ context.Context, req plugin.InitializeRequest) (plugin.InitializeResult, error) {
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
			Label: "pi-coding-agent",
			Name:  "ero-plugin-pi-coding-agent",
			Capabilities: plugin.ReviewProviderCapabilities{
				LoadRemoteComments: false,
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

func (p piProvider) DetectContext(_ context.Context, req plugin.DetectContextRequest) (plugin.DetectContextResult, error) {
	sessions, err := p.loadBridgeSessions()
	if err != nil {
		return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: false, Reason: err.Error()}}, nil
	}
	if _, ok := selectBridgeSession(sessions, req.Context.Repository, p.env("PI_CODING_AGENT_SESSION_ID")); ok {
		return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: true, Reason: "active pi-coding-agent bridge session detected"}}, nil
	}
	return plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: false, Reason: "no active pi-coding-agent bridge session for this repository"}}, nil
}

func (p piProvider) LoadRemoteThreads(_ context.Context, _ plugin.LoadRemoteThreadsRequest) (plugin.LoadRemoteThreadsResult, error) {
	return plugin.LoadRemoteThreadsResult{}, plugin.NewError(plugin.ErrorUnsupportedCapability, "pi-coding-agent does not load remote review comments")
}

func (p piProvider) PublishReview(ctx context.Context, req plugin.PublishReviewParams) (plugin.PublishReviewResultData, error) {
	sessions, err := p.loadBridgeSessions()
	if err != nil {
		return plugin.PublishReviewResultData{}, plugin.NewError(plugin.ErrorAuthRequired, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, ok := selectBridgeSession(sessions, req.Payload.Context.Repository, p.env("PI_CODING_AGENT_SESSION_ID"))
	if !ok {
		return plugin.PublishReviewResultData{}, plugin.NewError(plugin.ErrorAuthRequired, "start pi-coding-agent with the Ero bridge extension loaded, or set PI_CODING_AGENT_SESSION_ID to an active bridge session")
	}
	if err := p.sendToBridge(ctx, session, formatReviewMessage(req.Payload)); err != nil {
		return plugin.PublishReviewResultData{}, err
	}
	return plugin.PublishReviewResultData{Result: plugin.ReviewPublishResult{
		ProviderID:       providerID,
		ExternalReviewID: "pi-coding-agent:" + session.SessionID + ":" + req.Payload.Context.Session.LocalReviewID,
		PublishedRefs:    []plugin.PublishedReviewCommentRef{},
	}}, nil
}

func (p piProvider) loadBridgeSessions() ([]bridgeSession, error) {
	path := p.statePath
	if path == "" {
		path = bridgeRegistryPath(p.env)
	}
	trustedDir := filepath.Dir(path)
	if p.statePath == "" {
		trustedDir = bridgeRuntimeDir(p.env)
	}
	if err := validateTrustedDir(trustedDir); err != nil {
		return nil, err
	}
	if err := validateTrustedFile(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pi-coding-agent bridge registry: %w", err)
	}
	var state bridgeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("invalid pi-coding-agent bridge registry: %w", err)
	}
	return state.Sessions, nil
}

func (p piProvider) sendToBridge(ctx context.Context, session bridgeSession, message string) error {
	if session.SocketPath == "" {
		return plugin.NewError(plugin.ErrorRemoteValidationFailed, "pi-coding-agent bridge session has no socket_path")
	}
	trustedDir := filepath.Dir(p.statePath)
	if p.statePath == "" {
		trustedDir = bridgeRuntimeDir(p.env)
	}
	if err := validateSocketPath(trustedDir, session.SocketPath); err != nil {
		return plugin.NewErrorf(plugin.ErrorRemoteValidationFailed, "invalid pi-coding-agent bridge socket: %v", err)
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", session.SocketPath)
	if err != nil {
		return plugin.NewErrorf(plugin.ErrorNetwork, "connect to pi-coding-agent bridge socket: %v", err)
	}
	defer func() { _ = conn.Close() }()
	deadline := time.Now().Add(10 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return plugin.NewErrorf(plugin.ErrorNetwork, "set pi-coding-agent bridge socket deadline: %v", err)
	}
	if err := json.NewEncoder(conn).Encode(bridgeReviewMessage{Token: session.Token, SessionID: session.SessionID, Message: message}); err != nil {
		return plugin.NewErrorf(plugin.ErrorNetwork, "send review to pi-coding-agent bridge: %v", err)
	}
	if unixConn, ok := conn.(*net.UnixConn); ok {
		if err := unixConn.CloseWrite(); err != nil {
			return plugin.NewErrorf(plugin.ErrorNetwork, "finish pi-coding-agent bridge request: %v", err)
		}
	}
	var response bridgeResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return plugin.NewErrorf(plugin.ErrorNetwork, "read pi-coding-agent bridge response: %v", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "unknown bridge error"
		}
		return plugin.NewError(plugin.ErrorRemoteValidationFailed, response.Error)
	}
	return nil
}

func selectBridgeSession(sessions []bridgeSession, repo plugin.RepositoryMetadata, preferredID string) (bridgeSession, bool) {
	if preferredID != "" {
		for _, session := range sessions {
			if session.SessionID == preferredID {
				return session, true
			}
		}
		return bridgeSession{}, false
	}
	candidates := candidatePaths(repo.RepoPath, repo.WorktreeRoot)
	matches := scoredBridgeSessions(sessions, candidates, repo)
	if len(matches) == 0 {
		return bridgeSession{}, false
	}
	if repo.HeadSHA != "" {
		headMatches := filterScoredSessions(matches, func(session bridgeSession) bool {
			return session.HeadSHA != "" && session.HeadSHA == repo.HeadSHA
		})
		if len(headMatches) > 0 {
			return bestScoredSession(headMatches)
		}
	}
	if repo.CurrentBranch != "" {
		branchMatches := filterScoredSessions(matches, func(session bridgeSession) bool {
			return session.CurrentBranch != "" && session.CurrentBranch == repo.CurrentBranch
		})
		if len(branchMatches) > 0 {
			return bestScoredSession(branchMatches)
		}
		branchlessMatches := filterScoredSessions(matches, func(session bridgeSession) bool {
			return session.CurrentBranch == ""
		})
		if len(branchlessMatches) > 0 {
			return bestScoredSession(branchlessMatches)
		}
		if len(matches) > 1 {
			return bridgeSession{}, false
		}
	}
	return bestScoredSession(matches)
}

type scoredBridgeSession struct {
	session bridgeSession
	score   int
}

func scoredBridgeSessions(sessions []bridgeSession, candidates []string, repo plugin.RepositoryMetadata) []scoredBridgeSession {
	matches := make([]scoredBridgeSession, 0, len(sessions))
	for _, session := range sessions {
		score := scoreBridgeSession(session, candidates, repo)
		if score >= 0 {
			matches = append(matches, scoredBridgeSession{session: session, score: score})
		}
	}
	return matches
}

func filterScoredSessions(matches []scoredBridgeSession, keep func(bridgeSession) bool) []scoredBridgeSession {
	filtered := make([]scoredBridgeSession, 0, len(matches))
	for _, match := range matches {
		if keep(match.session) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func bestScoredSession(matches []scoredBridgeSession) (bridgeSession, bool) {
	if len(matches) == 0 {
		return bridgeSession{}, false
	}
	best := matches[0]
	for _, match := range matches[1:] {
		if match.score > best.score || (match.score == best.score && match.session.UpdatedAt > best.session.UpdatedAt) {
			best = match
		}
	}
	return best.session, true
}

func scoreBridgeSession(session bridgeSession, candidates []string, repo plugin.RepositoryMetadata) int {
	score := pathMatchScore(session, candidates)
	if score < 0 {
		return -1
	}
	if repo.CurrentBranch != "" && session.CurrentBranch != "" && repo.CurrentBranch == session.CurrentBranch {
		score += 4
	}
	if repo.HeadSHA != "" && session.HeadSHA != "" && repo.HeadSHA == session.HeadSHA {
		score += 8
	}
	return score
}

func pathMatchScore(session bridgeSession, candidates []string) int {
	best := -1
	for _, candidate := range candidates {
		if samePath(session.WorktreeRoot, candidate) && best < 2 {
			best = 2
		}
		if samePath(session.Cwd, candidate) && best < 1 {
			best = 1
		}
	}
	return best
}

func candidatePaths(repoPath, worktreeRoot string) []string {
	paths := make([]string, 0, 2)
	if worktreeRoot != "" {
		paths = append(paths, worktreeRoot)
	}
	if repoPath != "" {
		if abs, err := filepath.Abs(repoPath); err == nil {
			paths = append(paths, abs)
		} else {
			paths = append(paths, repoPath)
		}
	}
	return paths
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return filepath.Clean(absA) == filepath.Clean(absB)
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func formatReviewMessage(payload plugin.ReviewPublishPayload) string {
	var b strings.Builder
	b.WriteString("Ero review published for this repository.\n\n")
	if payload.Draft.Summary != "" {
		b.WriteString("Summary:\n")
		b.WriteString(payload.Draft.Summary)
		b.WriteString("\n\n")
	}
	if payload.Draft.Decision != "" {
		fmt.Fprintf(&b, "Decision: %s\n\n", payload.Draft.Decision)
	}
	if len(payload.Draft.Comments) == 0 {
		b.WriteString("No inline comments.")
		return b.String()
	}
	b.WriteString("Inline comments:\n")
	for _, comment := range payload.Draft.Comments {
		fmt.Fprintf(&b, "- %s%s — %s\n", comment.FilePath, formatLineRange(comment.Range), strings.TrimSpace(comment.Body))
	}
	return b.String()
}

func formatLineRange(lineRange plugin.ReviewLineRange) string {
	if lineRange.Start.NewLineNumber > 0 {
		if lineRange.End.NewLineNumber > 0 && lineRange.End.NewLineNumber != lineRange.Start.NewLineNumber {
			return fmt.Sprintf(":%d-%d", lineRange.Start.NewLineNumber, lineRange.End.NewLineNumber)
		}
		return fmt.Sprintf(":%d", lineRange.Start.NewLineNumber)
	}
	if lineRange.Start.OldLineNumber > 0 {
		if lineRange.End.OldLineNumber > 0 && lineRange.End.OldLineNumber != lineRange.Start.OldLineNumber {
			return fmt.Sprintf(":%d-%d", lineRange.Start.OldLineNumber, lineRange.End.OldLineNumber)
		}
		return fmt.Sprintf(":%d", lineRange.Start.OldLineNumber)
	}
	return ""
}

func bridgeRegistryPath(getenv func(string) string) string {
	return filepath.Join(bridgeRuntimeDir(getenv), bridgeStateFile)
}

func bridgeRuntimeDir(getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	if base := getenv("XDG_RUNTIME_DIR"); base != "" {
		return filepath.Join(base, bridgeRelDir)
	}
	if base := getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "ero", "runtime", bridgeRelDir)
	}
	if base, err := os.UserCacheDir(); err == nil && base != "" {
		return filepath.Join(base, "ero", "runtime", bridgeRelDir)
	}
	return filepath.Join(os.TempDir(), "ero-"+fmt.Sprint(os.Geteuid()), bridgeRelDir)
}

func validateTrustedDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat pi-coding-agent bridge directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("pi-coding-agent bridge path is not a directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("pi-coding-agent bridge directory is accessible by group or others")
	}
	return validateOwner(info)
}

func validateTrustedFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("pi-coding-agent bridge registry not found; load the pi-coding-agent bridge extension first: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("pi-coding-agent bridge registry is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("pi-coding-agent bridge registry is accessible by group or others")
	}
	return validateOwner(info)
}

func validateSocketPath(trustedDir, socketPath string) error {
	absDir, err := filepath.Abs(trustedDir)
	if err != nil {
		return err
	}
	absSocket, err := filepath.Abs(socketPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absDir, absSocket)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("socket_path is outside the trusted bridge directory")
	}
	info, err := os.Lstat(absSocket)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("socket_path is not a Unix socket")
	}
	return validateOwner(info)
}

func validateOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify owner")
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("owner does not match current user")
	}
	return nil
}

func (p piProvider) env(key string) string {
	if p.getenv != nil {
		return p.getenv(key)
	}
	return os.Getenv(key)
}
