package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// openAIUpstreamResponseFailedHandlerPrefix mirrors the literal that
// internal/handler.openAIForwardErrorAlreadyCommunicated matches with
// strings.HasPrefix. Keeping it here as a constant makes the cross-package
// coupling explicit and testable from this side of the boundary.
const openAIUpstreamResponseFailedHandlerPrefix = "upstream response failed:"

// A response.failed terminal event is already delivered to the client by the
// time this error surfaces. The handler suppresses its fallback error frame
// only when it recognizes the message by prefix; if the sentinel's text drifts
// (e.g. gains an "openai " qualifier), the handler stops recognizing it and
// appends a spurious 502 frame after a complete response.failed — corrupting
// the stream for strict SDKs.
//
// Asserting with Contains instead of HasPrefix would NOT catch that drift.
func TestOpenAIUpstreamResponseFailedKeepsHandlerPrefix(t *testing.T) {
	err := fmt.Errorf("%w: %s", errOpenAIUpstreamResponseFailed, "This content was flagged")

	require.Truef(t,
		strings.HasPrefix(err.Error(), openAIUpstreamResponseFailedHandlerPrefix),
		"sentinel message %q must keep the %q prefix matched by openAIForwardErrorAlreadyCommunicated",
		err.Error(), openAIUpstreamResponseFailedHandlerPrefix)

	// The sentinel must stay matchable by identity too — Forward uses
	// errors.Is to skip settling protocol-complete failures.
	require.True(t, errors.Is(err, errOpenAIUpstreamResponseFailed))

	require.Equal(t, "upstream response failed: This content was flagged", err.Error())
}
