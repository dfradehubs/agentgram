package slack

import (
	"errors"
	"net"
	"strings"
	"syscall"
)

const (
	errNoPermission    = ":lock: You don't have permission to use this agent."
	errTimeout         = ":hourglass: The operation took too long. Please try again."
	errInternal        = ":gear: An error occurred while processing your message."
	errAgentUnavail    = ":warning: The agent is currently unavailable."
	errOverloaded      = ":traffic_light: The system is processing many requests. Please try again in a few seconds."
	errPayloadTooLarge = ":package: The message is too large to process."
)

// classifyError returns a user-friendly error message.
func classifyError(err error) string {
	if err == nil {
		return errInternal
	}
	msg := err.Error()

	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(msg, "connection refused") {
		return errAgentUnavail
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errTimeout
	}
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "timeout") {
		return errTimeout
	}
	if strings.Contains(msg, "overloaded") || strings.Contains(msg, "rate") {
		return errOverloaded
	}
	if strings.Contains(msg, "too large") || strings.Contains(msg, "payload") {
		return errPayloadTooLarge
	}
	return errInternal
}
