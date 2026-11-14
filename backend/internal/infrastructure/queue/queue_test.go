package queue

import (
	"errors"
	"fmt"
	"testing"
	"unicode/utf8"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestPermanent(t *testing.T) {
	base := errors.New("bad payload")
	err := fmt.Errorf("handler: %w", Permanent(base))

	if !IsPermanent(err) {
		t.Error("wrapped permanent error not detected")
	}
	if !errors.Is(err, base) {
		t.Error("Permanent must keep the cause unwrappable")
	}
	if IsPermanent(base) {
		t.Error("plain error reported as permanent")
	}
	if Permanent(nil) != nil {
		t.Error("Permanent(nil) must be nil")
	}
}

func TestMessageDecode_MalformedBodyIsPermanent(t *testing.T) {
	var v struct{ ID string }
	if err := (Message{Topic: "t", Body: []byte(`{"ID":"x"}`)}).Decode(&v); err != nil || v.ID != "x" {
		t.Fatalf("Decode valid body: %v, %+v", err, v)
	}
	if err := (Message{Topic: "t", Body: []byte(`{`)}).Decode(&v); !IsPermanent(err) {
		t.Fatalf("Decode malformed body error = %v, want permanent", err)
	}
}

func TestAttemptOf(t *testing.T) {
	tests := []struct {
		headers amqp.Table
		want    int
	}{
		{nil, 1},
		{amqp.Table{headerAttempt: int32(3)}, 3},
		{amqp.Table{headerAttempt: int64(4)}, 4},
		{amqp.Table{headerAttempt: "2"}, 2},
		{amqp.Table{headerAttempt: int32(0)}, 1},
		{amqp.Table{headerAttempt: "garbage"}, 1},
	}
	for _, tt := range tests {
		if got := attemptOf(tt.headers); got != tt.want {
			t.Errorf("attemptOf(%v) = %d, want %d", tt.headers, got, tt.want)
		}
	}
}

func TestTruncate_KeepsValidUTF8(t *testing.T) {
	s := "ab€€" // 2 + 3 + 3 bytes
	for n := 0; n <= len(s)+1; n++ {
		got := truncate(s, n)
		if !utf8.ValidString(got) || len(got) > n {
			t.Errorf("truncate(%q, %d) = %q", s, n, got)
		}
	}
	if got := truncate(s, 4); got != "ab" {
		t.Errorf("truncate(%q, 4) = %q, want \"ab\"", s, got)
	}
}

func TestIsPreconditionFailed(t *testing.T) {
	err := fmt.Errorf("queue: declare queue x.retry: %w", &amqp.Error{Code: amqp.PreconditionFailed})
	if !isPreconditionFailed(err) {
		t.Error("precondition failure not detected")
	}
	if isPreconditionFailed(&amqp.Error{Code: amqp.ChannelError}) || isPreconditionFailed(errors.New("x")) {
		t.Error("other errors reported as precondition failure")
	}
}
