// Package pluginadapter provides the subprocess-based implementation of
// ports.ReviewProviderClient. It converts between internal/core domain types
// and pkg/plugin/protocol wire types at the process boundary.
package pluginadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"ero/internal/core"
	"ero/internal/ports"
	"ero/pkg/plugin/protocol"
)

// DefaultPluginTimeout is the per-request timeout used when none is configured.
const DefaultPluginTimeout = 30 * time.Second

// Client implements ports.ReviewProviderClient by spawning a subprocess and
// communicating over JSON-lines on stdin/stdout.
type Client struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	decoder        *json.Decoder
	stderr         *syncBuffer
	mu             sync.Mutex
	nextID         int
	timeout        time.Duration
	contributionID string
	closed         bool
}

// NewClient starts the plugin subprocess and returns a ready-to-use client.
// The command is executed in dir. stderr is captured for diagnostics.
func NewClient(command string, args []string, dir string, timeout time.Duration) (*Client, error) {
	return NewClientForContribution(command, args, dir, "", timeout)
}

// NewClientForContribution starts a plugin subprocess bound to a specific
// manifest contribution. The contribution ID is sent during initialize so a
// contribution-oriented plugin can expose the correct provider instance.
func NewClientForContribution(command string, args []string, dir, contributionID string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = DefaultPluginTimeout
	}

	cmd := exec.Command(command, args...)
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdout pipe: %w", err)
	}

	// Capture stderr for diagnostics.
	stderrBuf := &syncBuffer{}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin start: %w", err)
	}

	return &Client{
		cmd:            cmd,
		stdin:          stdin,
		decoder:        json.NewDecoder(bufio.NewReader(stdout)),
		stderr:         stderrBuf,
		timeout:        timeout,
		contributionID: contributionID,
	}, nil
}

// Initialize performs the protocol handshake with the plugin.
func (c *Client) Initialize(ctx context.Context) (core.ReviewProviderInfo, error) {
	req := protocol.InitializeRequest{Protocol: protocol.ProtocolVersion, ContributionID: c.contributionID}
	var result protocol.InitializeResult

	if err := c.call(ctx, "initialize", req, &result); err != nil {
		return core.ReviewProviderInfo{}, err
	}
	if result.Protocol != protocol.ProtocolVersion {
		return core.ReviewProviderInfo{}, fmt.Errorf("plugin protocol mismatch: expected %q, got %q", protocol.ProtocolVersion, result.Protocol)
	}

	return toCoreProviderInfo(result.Provider), nil
}

// DetectContext asks the plugin whether it considers the review context
// applicable.
func (c *Client) DetectContext(ctx context.Context, review core.ReviewContext) (core.DetectionResult, error) {
	req := protocol.DetectContextRequest{Context: toProtocolReviewContext(review)}
	var result protocol.DetectContextResult

	if err := c.call(ctx, "detect_context", req, &result); err != nil {
		return core.DetectionResult{}, err
	}

	return core.DetectionResult{
		Applicable: result.Result.Applicable,
		Reason:     result.Result.Reason,
	}, nil
}

// LoadRemoteThreads fetches remote review threads from the plugin.
func (c *Client) LoadRemoteThreads(ctx context.Context, review core.ReviewContext) ([]core.RemoteReviewThread, error) {
	req := protocol.LoadRemoteThreadsRequest{Context: toProtocolReviewContext(review)}
	var result protocol.LoadRemoteThreadsResult

	if err := c.call(ctx, "load_remote_threads", req, &result); err != nil {
		return nil, err
	}

	threads := make([]core.RemoteReviewThread, len(result.Threads))
	for i, t := range result.Threads {
		threads[i] = toCoreRemoteThread(t)
	}
	return threads, nil
}

// PublishReview sends the draft to the plugin for publication.
func (c *Client) PublishReview(ctx context.Context, request core.PublishReviewRequest) (core.PublishReviewResult, error) {
	params := protocol.PublishReviewParams{
		Payload: protocol.ReviewPublishPayload{
			ProviderID: request.ProviderID,
			Context:    toProtocolReviewContext(request.Context),
			Draft:      toProtocolDraftSnapshot(request.Draft),
		},
	}
	var result protocol.PublishReviewResultData

	if err := c.call(ctx, "publish_review", params, &result); err != nil {
		return core.PublishReviewResult{}, err
	}

	return toCorePublishResult(result.Result), nil
}

// Close shuts down the plugin subprocess. It closes stdin and waits for the
// process to exit.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close stdin to signal EOF to the plugin's server loop.
	if c.closed {
		return nil
	}
	c.closed = true

	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}

	if c.cmd != nil && c.cmd.Process != nil {
		// Give the process a moment to exit gracefully after stdin close.
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()

		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
			return fmt.Errorf("plugin process killed after timeout")
		}
	}

	return nil
}

// Stderr returns the plugin's captured stderr output. Useful for diagnostics
// when a plugin returns an error.
func (c *Client) Stderr() string {
	if c.stderr == nil {
		return ""
	}
	return c.stderr.String()
}

// call sends a JSON-lines request, reads the response, and handles errors.
func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.stdin == nil {
		return fmt.Errorf("plugin client is closed")
	}

	c.nextID++
	id := strconv.Itoa(c.nextID)

	req := protocol.Request{
		ID:     id,
		Method: method,
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("plugin marshal params: %w", err)
	}
	req.Params = paramsBytes

	// Write request as a single JSON line.
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plugin marshal request: %w", err)
	}
	reqBytes = append(reqBytes, '\n')

	writeCh := make(chan error, 1)
	go func() {
		_, err := c.stdin.Write(reqBytes)
		if err != nil {
			writeCh <- fmt.Errorf("plugin write: %w", err)
			return
		}
		writeCh <- nil
	}()
	select {
	case <-ctx.Done():
		c.markUnusableLocked()
		return fmt.Errorf("plugin request timed out while writing: %w", ctx.Err())
	case err := <-writeCh:
		if err != nil {
			return err
		}
	}

	// Read response with context timeout.
	type decodeResult struct {
		resp protocol.Response
		err  error
	}
	ch := make(chan decodeResult, 1)

	go func() {
		var resp protocol.Response
		if err := c.decoder.Decode(&resp); err != nil {
			ch <- decodeResult{err: fmt.Errorf("plugin decode response: %w", err)}
			return
		}
		ch <- decodeResult{resp: resp}
	}()

	select {
	case <-ctx.Done():
		c.markUnusableLocked()
		return fmt.Errorf("plugin request timed out: %w", ctx.Err())
	case dr := <-ch:
		if dr.err != nil {
			c.markUnusableLocked()
			return dr.err
		}
		resp := dr.resp

		// Check for ID mismatch.
		if resp.ID != id {
			c.markUnusableLocked()
			return fmt.Errorf("plugin response id mismatch: expected %q, got %q", id, resp.ID)
		}

		// Check for protocol error.
		if resp.Error != nil {
			return toCoreProviderError(resp.Error)
		}

		// Decode the result into the caller's type.
		resultBytes, err := json.Marshal(resp.Result)
		if err != nil {
			return fmt.Errorf("plugin re-marshal result: %w", err)
		}
		if err := json.Unmarshal(resultBytes, result); err != nil {
			c.markUnusableLocked()
			return fmt.Errorf("plugin unmarshal result: %w", err)
		}

		return nil
	}
}

func (c *Client) markUnusableLocked() {
	if c.closed {
		return
	}
	c.closed = true
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		go func(cmd *exec.Cmd) { _ = cmd.Wait() }(c.cmd)
	}
}

// Ensure Client implements ports.ReviewProviderClient.
var _ ports.ReviewProviderClient = (*Client)(nil)

// ---------- conversion helpers ----------

func toCoreProviderError(err *protocol.Error) error {
	if err == nil {
		return nil
	}
	kind := core.ProviderErrorInternal
	switch err.Code {
	case protocol.ErrorAuthRequired:
		kind = core.ProviderErrorAuthenticationRequired
	case protocol.ErrorNotApplicable:
		kind = core.ProviderErrorNotApplicable
	case protocol.ErrorUnsupportedCapability:
		kind = core.ProviderErrorUnsupportedCapability
	case protocol.ErrorRemoteRateLimited:
		kind = core.ProviderErrorRateLimited
	case protocol.ErrorNetwork:
		kind = core.ProviderErrorTransientNetwork
	case protocol.ErrorRemoteValidationFailed, protocol.ErrorInvalidRequest:
		kind = core.ProviderErrorRemoteValidation
	case protocol.ErrorInternal, protocol.ErrorPartialPublishUnknown:
		kind = core.ProviderErrorInternal
	}
	return core.NewProviderError(kind, err.Error(), err)
}

func toCoreProviderInfo(info protocol.ReviewProviderInfo) core.ReviewProviderInfo {
	decisions := make([]core.ReviewDecision, len(info.Capabilities.Decisions))
	for i, d := range info.Capabilities.Decisions {
		decisions[i] = core.ReviewDecision(d)
	}
	return core.ReviewProviderInfo{
		ID:    info.ID,
		Label: info.Label,
		Name:  info.Name,
		Capabilities: core.ReviewProviderCapabilities{
			LoadRemoteComments: info.Capabilities.LoadRemoteComments,
			PublishReview:      info.Capabilities.PublishReview,
			Decisions:          decisions,
			IdempotentPublish:  info.Capabilities.IdempotentPublish,
		},
	}
}

func toProtocolReviewContext(ctx core.ReviewContext) protocol.ReviewContext {
	return protocol.ReviewContext{
		Repository: toProtocolRepository(ctx.Repository),
		Target:     toProtocolTarget(ctx.Target),
		Diff:       toProtocolDiff(ctx.Diff),
		Files:      toProtocolFiles(ctx.Files),
		Session:    toProtocolSession(ctx.Session),
	}
}

func toProtocolRepository(r core.RepositoryMetadata) protocol.RepositoryMetadata {
	remotes := make([]protocol.GitRemote, len(r.Remotes))
	for i, rem := range r.Remotes {
		remotes[i] = protocol.GitRemote{Name: rem.Name, URL: rem.URL}
	}
	return protocol.RepositoryMetadata{
		RepoPath:      r.RepoPath,
		WorktreeRoot:  r.WorktreeRoot,
		Remotes:       remotes,
		DefaultBranch: r.DefaultBranch,
		CurrentBranch: r.CurrentBranch,
		HeadSHA:       r.HeadSHA,
	}
}

func toProtocolTarget(t core.ReviewTargetMetadata) protocol.ReviewTargetMetadata {
	return protocol.ReviewTargetMetadata{
		Mode:         string(t.Mode),
		BaseRef:      t.BaseRef,
		HeadRef:      t.HeadRef,
		BaseSHA:      t.BaseSHA,
		HeadSHA:      t.HeadSHA,
		MergeBaseSHA: t.MergeBaseSHA,
	}
}

func toProtocolDiff(d core.DiffMetadata) protocol.DiffMetadata {
	return protocol.DiffMetadata{
		FilesChanged: d.FilesChanged,
		Additions:    d.Additions,
		Deletions:    d.Deletions,
	}
}

func toProtocolFiles(files []core.ReviewFileMetadata) []protocol.ReviewFileMetadata {
	if files == nil {
		return nil
	}
	out := make([]protocol.ReviewFileMetadata, len(files))
	for i, f := range files {
		out[i] = protocol.ReviewFileMetadata{
			Path:        f.Path,
			OldPath:     f.OldPath,
			Status:      string(f.Status),
			Language:    f.Language,
			Hunks:       toProtocolHunks(f.Hunks),
			LineAnchors: toProtocolLineAnchors(f.LineAnchors),
		}
	}
	return out
}

func toProtocolHunks(hunks []core.ReviewHunkAnchor) []protocol.ReviewHunkAnchor {
	if hunks == nil {
		return nil
	}
	out := make([]protocol.ReviewHunkAnchor, len(hunks))
	for i, h := range hunks {
		out[i] = protocol.ReviewHunkAnchor{
			SectionID:    h.SectionID,
			OldStartLine: h.OldStartLine,
			NewStartLine: h.NewStartLine,
		}
	}
	return out
}

func toProtocolLineAnchors(anchors []core.ReviewLineAnchor) []protocol.ReviewLineAnchor {
	if anchors == nil {
		return nil
	}
	out := make([]protocol.ReviewLineAnchor, len(anchors))
	for i, a := range anchors {
		out[i] = protocol.ReviewLineAnchor{
			FilePath:      a.FilePath,
			OldLineNumber: a.OldLineNumber,
			NewLineNumber: a.NewLineNumber,
			Side:          string(a.Side),
		}
	}
	return out
}

func toProtocolSession(s core.ReviewSessionMetadata) protocol.ReviewSessionMetadata {
	return protocol.ReviewSessionMetadata{
		EroVersion:      s.EroVersion,
		ProtocolVersion: s.ProtocolVersion,
		LocalReviewID:   s.LocalReviewID,
		CreatedAt:       s.CreatedAt,
		IdempotencyKey:  s.IdempotencyKey,
	}
}

func toProtocolDraftSnapshot(s core.ReviewDraftSnapshot) protocol.ReviewDraftSnapshot {
	comments := make([]protocol.ReviewComment, len(s.Comments))
	for i, c := range s.Comments {
		refs := make([]protocol.ProviderCommentRef, len(c.ProviderRefs))
		for j, r := range c.ProviderRefs {
			refs[j] = protocol.ProviderCommentRef{
				ProviderID:  r.ProviderID,
				ExternalID:  r.ExternalID,
				ExternalURL: r.ExternalURL,
			}
		}
		comments[i] = protocol.ReviewComment{
			ID:           c.ID,
			FilePath:     c.FilePath,
			Range:        toProtocolLineRange(c.Range),
			Body:         c.Body,
			State:        string(c.State),
			ProviderRefs: refs,
		}
	}
	return protocol.ReviewDraftSnapshot{
		ID:             s.ID,
		Comments:       comments,
		Decision:       protocol.ReviewDecision(s.Decision),
		Summary:        s.Summary,
		IdempotencyKey: s.IdempotencyKey,
	}
}

func toProtocolLineRange(r core.ReviewLineRange) protocol.ReviewLineRange {
	return protocol.ReviewLineRange{
		Start: protocol.ReviewLineRef{
			OldLineNumber: r.Start.OldLineNumber,
			NewLineNumber: r.Start.NewLineNumber,
			Kind:          string(r.Start.Kind),
		},
		End: protocol.ReviewLineRef{
			OldLineNumber: r.End.OldLineNumber,
			NewLineNumber: r.End.NewLineNumber,
			Kind:          string(r.End.Kind),
		},
	}
}

func toCoreRemoteThread(t protocol.RemoteReviewThread) core.RemoteReviewThread {
	comments := make([]core.RemoteReviewComment, len(t.Comments))
	for i, c := range t.Comments {
		comments[i] = core.RemoteReviewComment{
			ExternalID: c.ExternalID,
			Author:     c.Author,
			Body:       c.Body,
			CreatedAt:  c.CreatedAt,
		}
	}
	return core.RemoteReviewThread{
		ProviderID:  t.ProviderID,
		ExternalID:  t.ExternalID,
		FilePath:    t.FilePath,
		Range:       toCoreLineRange(t.Range),
		Comments:    comments,
		Unmapped:    t.Unmapped,
		ExternalURL: t.ExternalURL,
	}
}

func toCoreLineRange(r protocol.ReviewLineRange) core.ReviewLineRange {
	return core.ReviewLineRange{
		Start: core.ReviewLineRef{
			OldLineNumber: r.Start.OldLineNumber,
			NewLineNumber: r.Start.NewLineNumber,
			Kind:          core.LineKind(r.Start.Kind),
		},
		End: core.ReviewLineRef{
			OldLineNumber: r.End.OldLineNumber,
			NewLineNumber: r.End.NewLineNumber,
			Kind:          core.LineKind(r.End.Kind),
		},
	}
}

func toCorePublishResult(r protocol.ReviewPublishResult) core.PublishReviewResult {
	refs := make([]core.PublishedReviewCommentRef, len(r.PublishedRefs))
	for i, ref := range r.PublishedRefs {
		refs[i] = core.PublishedReviewCommentRef{
			LocalCommentID: ref.LocalCommentID,
			ExternalID:     ref.ExternalID,
			ExternalURL:    ref.ExternalURL,
		}
	}
	return core.PublishReviewResult{
		ProviderID:       r.ProviderID,
		ExternalReviewID: r.ExternalReviewID,
		ExternalURL:      r.ExternalURL,
		PublishedRefs:    refs,
		Ambiguous:        r.Ambiguous,
	}
}

// syncBuffer is a simple thread-safe byte buffer for capturing stderr.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
