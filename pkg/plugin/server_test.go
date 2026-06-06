package plugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ero/pkg/plugin"
)

// fakeProvider implements plugin.ReviewProvider for tests.
type fakeProvider struct {
	initResult    plugin.InitializeResult
	initErr       error
	detectResult  plugin.DetectContextResult
	detectErr     error
	threadsResult plugin.LoadRemoteThreadsResult
	threadsErr    error
	publishResult plugin.PublishReviewResultData
	publishErr    error
	methodsCalled []string
}

type fakeSnapshotProvider struct {
	*fakeProvider
	snapshotResult plugin.LoadRemoteSnapshotResult
	snapshotErr    error
}

func (f *fakeSnapshotProvider) LoadRemoteSnapshot(_ context.Context, _ plugin.LoadRemoteSnapshotRequest) (plugin.LoadRemoteSnapshotResult, error) {
	f.methodsCalled = append(f.methodsCalled, "load_remote_snapshot")
	return f.snapshotResult, f.snapshotErr
}

func (f *fakeProvider) Initialize(_ context.Context, _ plugin.InitializeRequest) (plugin.InitializeResult, error) {
	f.methodsCalled = append(f.methodsCalled, "initialize")
	return f.initResult, f.initErr
}

func (f *fakeProvider) DetectContext(_ context.Context, _ plugin.DetectContextRequest) (plugin.DetectContextResult, error) {
	f.methodsCalled = append(f.methodsCalled, "detect_context")
	return f.detectResult, f.detectErr
}

func (f *fakeProvider) LoadRemoteThreads(_ context.Context, _ plugin.LoadRemoteThreadsRequest) (plugin.LoadRemoteThreadsResult, error) {
	f.methodsCalled = append(f.methodsCalled, "load_remote_threads")
	return f.threadsResult, f.threadsErr
}

func (f *fakeProvider) PublishReview(_ context.Context, _ plugin.PublishReviewParams) (plugin.PublishReviewResultData, error) {
	f.methodsCalled = append(f.methodsCalled, "publish_review")
	return f.publishResult, f.publishErr
}

func TestServeReviewProviderInitialize(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		initResult: plugin.InitializeResult{
			Protocol: plugin.ProtocolVersion,
			Provider: plugin.ReviewProviderInfo{
				ID:           "fake",
				Label:        "Fake Provider",
				Capabilities: plugin.ReviewProviderCapabilities{PublishReview: true},
			},
		},
	}

	input := bytes.NewBufferString(`{"id":"1","method":"initialize","params":{"protocol":"ero.plugin.v1"}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json response: %v\nraw: %s", err, output.String())
	}

	if string(response["id"]) != `"1"` {
		t.Fatalf("expected id 1, got %s", response["id"])
	}
	if _, ok := response["result"]; !ok {
		t.Fatalf("expected result, got error or nil: %s", output.String())
	}
	if _, ok := response["error"]; ok {
		t.Fatalf("unexpected error field: %s", response["error"])
	}
}

func TestServeReviewProviderInitializeProtocolMismatch(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		initErr: plugin.NewError(plugin.ErrorInvalidRequest, "unsupported protocol"),
	}

	input := bytes.NewBufferString(`{"id":"1","method":"initialize","params":{"protocol":"v999"}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if response.ID != "1" {
		t.Fatalf("expected id 1, got %s", response.ID)
	}
	if response.Error == nil {
		t.Fatalf("expected error, got result")
	}
	if response.Error.Code != plugin.ErrorInvalidRequest {
		t.Fatalf("expected error code %q, got %q", plugin.ErrorInvalidRequest, response.Error.Code)
	}
}

func TestServeReviewProviderInternalErrorConversion(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		initErr: context.DeadlineExceeded, // ordinary Go error, not a protocol error
	}

	input := bytes.NewBufferString(`{"id":"2","method":"initialize","params":{"protocol":"ero.plugin.v1"}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if response.Error == nil {
		t.Fatalf("expected error, got result")
	}
	if response.Error.Code != plugin.ErrorInternal {
		t.Fatalf("expected %q for ordinary error, got %q", plugin.ErrorInternal, response.Error.Code)
	}
}

func TestServeReviewProviderWrappedTypedErrorPreserved(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		detectErr: fmt.Errorf("detect failed: %w", plugin.NewError(plugin.ErrorNotApplicable, "not a Go repo")),
	}

	input := bytes.NewBufferString(`{"id":"3w","method":"detect_context","params":{"context":{"repository":{"repo_path":"."}}}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if response.Error == nil {
		t.Fatalf("expected error, got result")
	}
	if response.Error.Code != plugin.ErrorNotApplicable {
		t.Fatalf("expected %q, got %q", plugin.ErrorNotApplicable, response.Error.Code)
	}
}

func TestServeReviewProviderTypedErrorPreserved(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		detectErr: plugin.NewError(plugin.ErrorNotApplicable, "not a Go repo"),
	}

	input := bytes.NewBufferString(`{"id":"3","method":"detect_context","params":{"context":{"repository":{"repo_path":"."}}}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if response.Error == nil {
		t.Fatalf("expected error, got result")
	}
	if response.Error.Code != plugin.ErrorNotApplicable {
		t.Fatalf("expected %q, got %q", plugin.ErrorNotApplicable, response.Error.Code)
	}
}

func TestServeReviewProviderUnknownMethod(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{}
	input := bytes.NewBufferString(`{"id":"4","method":"do_magic","params":{}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if response.Error == nil {
		t.Fatalf("expected error for unknown method")
	}
	if response.Error.Code != plugin.ErrorUnsupportedCapability {
		t.Fatalf("expected %q, got %q", plugin.ErrorUnsupportedCapability, response.Error.Code)
	}
}

func TestServeReviewProviderMalformedJSON(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{}
	input := bytes.NewBufferString("not json\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if response.Error == nil {
		t.Fatalf("expected error for malformed JSON")
	}
	if response.Error.Code != plugin.ErrorInvalidRequest {
		t.Fatalf("expected %q, got %q", plugin.ErrorInvalidRequest, response.Error.Code)
	}
}

func TestServeReviewProviderLargeRequestLine(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		detectResult: plugin.DetectContextResult{Result: plugin.DetectionResult{Applicable: true}},
	}

	largePath := strings.Repeat("a", 128*1024)
	input := bytes.NewBufferString(`{"id":"large","method":"detect_context","params":{"context":{"repository":{"repo_path":"` + largePath + `"}}}}` + "\n")
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, input, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	var response plugin.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if response.ID != "large" || response.Error != nil {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestLoadRemoteSnapshotDispatchesOptionalMethod(t *testing.T) {
	provider := &fakeSnapshotProvider{fakeProvider: &fakeProvider{}, snapshotResult: plugin.LoadRemoteSnapshotResult{RuntimeProviderID: "github", Overview: &plugin.ProviderOverview{Title: "PR"}}}
	input := bytes.NewBufferString(`{"id":"s1","method":"load_remote_snapshot","params":{"context":{"repository":{"repo_path":"."}}}}` + "\n")
	var output bytes.Buffer
	if err := plugin.ServeReviewProvider(context.Background(), provider, input, &output); err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}
	var response struct {
		ID     string                          `json:"id"`
		Result plugin.LoadRemoteSnapshotResult `json:"result"`
		Error  *plugin.Error                   `json:"error,omitempty"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("unexpected error: %v raw=%s", response.Error, output.String())
	}
	if response.Result.Overview == nil {
		t.Fatalf("missing overview in result: %#v raw=%s", response.Result, output.String())
	}
	if response.Result.Overview.Title != "PR" {
		t.Fatalf("unexpected overview title: got %q, want %q", response.Result.Overview.Title, "PR")
	}
	if !reflect.DeepEqual(provider.methodsCalled, []string{"load_remote_snapshot"}) {
		t.Fatalf("expected load_remote_snapshot call, got %v", provider.methodsCalled)
	}
}

func TestLoadRemoteSnapshotUnsupportedKeepsOldProvidersValid(t *testing.T) {
	provider := &fakeProvider{threadsResult: plugin.LoadRemoteThreadsResult{Threads: []plugin.RemoteReviewThread{{ProviderID: "old", ExternalID: "t1"}}}}
	input := bytes.NewBufferString(strings.Join([]string{
		`{"id":"s1","method":"load_remote_snapshot","params":{"context":{"repository":{"repo_path":"."}}}}`,
		`{"id":"t1","method":"load_remote_threads","params":{"context":{"repository":{"repo_path":"."}}}}`,
	}, "\n") + "\n")
	var output bytes.Buffer
	if err := plugin.ServeReviewProvider(context.Background(), provider, input, &output); err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}
	dec := json.NewDecoder(&output)
	var unsupported plugin.Response
	if err := dec.Decode(&unsupported); err != nil {
		t.Fatalf("decode unsupported: %v", err)
	}
	if unsupported.Error == nil || unsupported.Error.Code != plugin.ErrorUnsupportedCapability {
		t.Fatalf("expected unsupported capability, got %#v", unsupported.Error)
	}
	var fallback struct {
		ID     string                         `json:"id"`
		Result plugin.LoadRemoteThreadsResult `json:"result"`
		Error  *plugin.Error                  `json:"error,omitempty"`
	}
	if err := dec.Decode(&fallback); err != nil {
		t.Fatalf("decode fallback: %v", err)
	}
	if fallback.Error != nil || len(fallback.Result.Threads) != 1 {
		t.Fatalf("expected fallback threads, got %#v", fallback)
	}
}

func TestServeReviewProviderRoundTripSuccess(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		initResult: plugin.InitializeResult{
			Protocol: plugin.ProtocolVersion,
			Provider: plugin.ReviewProviderInfo{
				ID: "roundtrip", Label: "RT",
				Capabilities: plugin.ReviewProviderCapabilities{
					PublishReview: true,
					Decisions:     []plugin.ReviewDecision{plugin.ReviewDecisionApprove},
				},
			},
		},
	}

	// Send a simple publish review request after init.
	lines := []string{
		`{"id":"10","method":"initialize","params":{"protocol":"ero.plugin.v1"}}`,
		`{"id":"11","method":"publish_review","params":{"payload":{"provider_id":"rt","context":{"repository":{"repo_path":"."}},"draft":{"id":"d1","comments":[],"idempotency_key":"k1"}}}}`,
	}

	var inputBuf bytes.Buffer
	for _, line := range lines {
		inputBuf.WriteString(line + "\n")
	}
	var output bytes.Buffer

	err := plugin.ServeReviewProvider(context.Background(), provider, &inputBuf, &output)
	if err != nil {
		t.Fatalf("ServeReviewProvider returned error: %v", err)
	}

	dec := json.NewDecoder(&output)
	// First response: init result
	var resp1 plugin.Response
	if err := dec.Decode(&resp1); err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}
	if resp1.ID != "10" || resp1.Error != nil {
		t.Fatalf("unexpected init response: id=%s error=%v", resp1.ID, resp1.Error)
	}

	// Second response: publish result
	var resp2 plugin.Response
	if err := dec.Decode(&resp2); err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}
	if resp2.ID != "11" || resp2.Error != nil {
		t.Fatalf("unexpected publish response: id=%s error=%v", resp2.ID, resp2.Error)
	}

	expectedMethods := []string{"initialize", "publish_review"}
	if !reflect.DeepEqual(provider.methodsCalled, expectedMethods) {
		t.Fatalf("expected methods %v, got %v", expectedMethods, provider.methodsCalled)
	}
}
