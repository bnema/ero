package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"ero/pkg/plugin/protocol"
)

const maxRequestLineBytes = 16 * 1024 * 1024

// ReviewProvider is the interface a plugin author implements to create a
// review provider plugin. Each method corresponds to a protocol method.
type ReviewProvider interface {
	Initialize(ctx context.Context, req InitializeRequest) (InitializeResult, error)
	DetectContext(ctx context.Context, req DetectContextRequest) (DetectContextResult, error)
	LoadRemoteThreads(ctx context.Context, req LoadRemoteThreadsRequest) (LoadRemoteThreadsResult, error)
	PublishReview(ctx context.Context, req PublishReviewParams) (PublishReviewResultData, error)
}

type RemoteSnapshotProvider interface {
	LoadRemoteSnapshot(ctx context.Context, req LoadRemoteSnapshotRequest) (LoadRemoteSnapshotResult, error)
}

// ServeReviewProvider runs the JSON-lines protocol server loop. It reads
// requests from stdin, dispatches to the provider, and writes responses to
// stdout. The server returns after EOF on stdin or when ctx is cancelled.
//
// Ordinary Go errors returned by the provider are converted to
// ErrorInternal. Typed *Error values are preserved so the host can act on
// structured error codes.
func ServeReviewProvider(ctx context.Context, provider ReviewProvider, stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64*1024), maxRequestLineBytes)
	encoder := json.NewEncoder(stdout)

	for scanner.Scan() {
		// Check context cancellation before processing each request.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var req protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = encoder.Encode(protocol.Response{
				ID:    "",
				Error: protocol.NewError(ErrorInvalidRequest, "failed to parse request: "+err.Error()),
			})
			continue
		}

		resp := dispatch(ctx, provider, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// dispatch routes a single request to the appropriate provider method and
// wraps the result into a Response envelope.
func dispatch(ctx context.Context, provider ReviewProvider, req protocol.Request) protocol.Response {
	switch req.Method {
	case "initialize":
		return handleInitialize(ctx, provider, req)
	case "detect_context":
		return handleDetectContext(ctx, provider, req)
	case "load_remote_threads":
		return handleLoadRemoteThreads(ctx, provider, req)
	case "load_remote_snapshot":
		return handleLoadRemoteSnapshot(ctx, provider, req)
	case "publish_review":
		return handlePublishReview(ctx, provider, req)
	default:
		return protocol.Response{
			ID:    req.ID,
			Error: protocol.NewError(ErrorUnsupportedCapability, "unknown method: "+req.Method),
		}
	}
}

func handleInitialize(ctx context.Context, provider ReviewProvider, req protocol.Request) protocol.Response {
	var params protocol.InitializeRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return protocol.Response{
			ID:    req.ID,
			Error: protocol.NewError(ErrorInvalidRequest, "invalid initialize params: "+err.Error()),
		}
	}

	result, err := provider.Initialize(ctx, params)
	if err != nil {
		return protocol.Response{ID: req.ID, Error: toProtocolError(err)}
	}

	return protocol.Response{ID: req.ID, Result: result}
}

func handleDetectContext(ctx context.Context, provider ReviewProvider, req protocol.Request) protocol.Response {
	var params protocol.DetectContextRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return protocol.Response{
			ID:    req.ID,
			Error: protocol.NewError(ErrorInvalidRequest, "invalid detect_context params: "+err.Error()),
		}
	}

	result, err := provider.DetectContext(ctx, params)
	if err != nil {
		return protocol.Response{ID: req.ID, Error: toProtocolError(err)}
	}

	return protocol.Response{ID: req.ID, Result: result}
}

func handleLoadRemoteThreads(ctx context.Context, provider ReviewProvider, req protocol.Request) protocol.Response {
	var params protocol.LoadRemoteThreadsRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return protocol.Response{
			ID:    req.ID,
			Error: protocol.NewError(ErrorInvalidRequest, "invalid load_remote_threads params: "+err.Error()),
		}
	}

	result, err := provider.LoadRemoteThreads(ctx, params)
	if err != nil {
		return protocol.Response{ID: req.ID, Error: toProtocolError(err)}
	}

	return protocol.Response{ID: req.ID, Result: result}
}

func handleLoadRemoteSnapshot(ctx context.Context, provider ReviewProvider, req protocol.Request) protocol.Response {
	snapshotProvider, ok := provider.(RemoteSnapshotProvider)
	if !ok {
		return protocol.Response{ID: req.ID, Error: protocol.NewError(ErrorUnsupportedCapability, "method not supported: load_remote_snapshot")}
	}
	var params protocol.LoadRemoteSnapshotRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return protocol.Response{ID: req.ID, Error: protocol.NewError(ErrorInvalidRequest, "invalid load_remote_snapshot params: "+err.Error())}
	}
	result, err := snapshotProvider.LoadRemoteSnapshot(ctx, params)
	if err != nil {
		return protocol.Response{ID: req.ID, Error: toProtocolError(err)}
	}
	return protocol.Response{ID: req.ID, Result: result}
}

func handlePublishReview(ctx context.Context, provider ReviewProvider, req protocol.Request) protocol.Response {
	var params protocol.PublishReviewParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return protocol.Response{
			ID:    req.ID,
			Error: protocol.NewError(ErrorInvalidRequest, "invalid publish_review params: "+err.Error()),
		}
	}

	result, err := provider.PublishReview(ctx, params)
	if err != nil {
		return protocol.Response{ID: req.ID, Error: toProtocolError(err)}
	}

	return protocol.Response{ID: req.ID, Result: result}
}

// toProtocolError converts a Go error to a protocol.Error. If err is already
// a *protocol.Error it is returned as-is; otherwise it is wrapped as an
// internal error.
func toProtocolError(err error) *protocol.Error {
	if pe := protocol.AsError(err); pe != nil {
		return pe
	}
	return protocol.NewError(ErrorInternal, err.Error())
}
