package streaming

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsAlreadyExists(t *testing.T) {
	if !isAlreadyExists(status.Error(codes.AlreadyExists, "resource already exists")) {
		t.Error("expected AlreadyExists to be detected")
	}
	if isAlreadyExists(status.Error(codes.Internal, "boom")) {
		t.Error("expected Internal not to be treated as AlreadyExists")
	}
	if isAlreadyExists(errors.New("plain error")) {
		t.Error("expected plain errors not to be treated as AlreadyExists")
	}
	if isAlreadyExists(nil) {
		t.Error("expected nil not to be treated as AlreadyExists")
	}
}
