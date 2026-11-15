package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
)

type legacyTemporaryError struct{}

func (legacyTemporaryError) Error() string   { return "legacy temporary error" }
func (legacyTemporaryError) Timeout() bool   { return false }
func (legacyTemporaryError) Temporary() bool { return true }

type timeoutOnlyError struct{}

func (timeoutOnlyError) Error() string { return "timeout" }
func (timeoutOnlyError) Timeout() bool { return true }

func TestTransientCheckErrorExplicitReasons(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, true},
		{"wrapped timeout", fmt.Errorf("request: %w", timeoutOnlyError{}), true},
		{"incomplete response", io.ErrUnexpectedEOF, true},
		{"wrapped reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"connection refused", syscall.ECONNREFUSED, true},
		{"connection aborted", syscall.ECONNABORTED, true},
		{"try again", syscall.EAGAIN, true},
		{"interrupted", syscall.EINTR, true},
		{"DNS timeout", &net.DNSError{IsTimeout: true}, true},
		{"temporary DNS", fmt.Errorf("lookup: %w", &net.DNSError{IsTemporary: true}), true},
		{"DNS not found", &net.DNSError{IsNotFound: true}, false},
		{"cancelled", context.Canceled, false},
		{"wrapped cancellation", fmt.Errorf("request: %w", context.Canceled), false},
		{"ordinary error", errors.New("bad response"), false},
		{"unspecified legacy temporary", legacyTemporaryError{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := transientCheckError(tc.err); got != tc.want {
				t.Fatalf("transientCheckError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type errorThenSuccessTransport struct {
	err   error
	calls int
}

func (t *errorThenSuccessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	if t.calls == 1 {
		return nil, t.err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
}

func TestGetRetriesOnlyExplicitNetworkFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		wantCalls int
	}{
		{"temporary DNS", &net.DNSError{IsTemporary: true}, 2},
		{"connection reset", syscall.ECONNRESET, 2},
		{"unspecified legacy temporary", legacyTemporaryError{}, 1},
		{"cancelled", context.Canceled, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &errorThenSuccessTransport{err: tc.err}
			source := &Source{Client: &http.Client{Transport: transport}}
			data, _, err := source.get(context.Background(), "https://example.invalid/release.json", "")
			if transport.calls != tc.wantCalls {
				t.Fatalf("made %d requests, want %d", transport.calls, tc.wantCalls)
			}
			if tc.wantCalls == 1 {
				if err == nil {
					t.Fatal("non-retryable error was accepted")
				}
			} else if err != nil || string(data) != "ok" {
				t.Fatalf("retry result: %q, %v", data, err)
			}
		})
	}
}
