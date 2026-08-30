package domain_test

import (
	"testing"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

func TestIdempotencyTag_Deterministic(t *testing.T) {
	followerID := uuid.New()
	tag1 := domain.IdempotencyTag(42, followerID)
	tag2 := domain.IdempotencyTag(42, followerID)
	if tag1 != tag2 {
		t.Fatalf("tag not deterministic: %q != %q", tag1, tag2)
	}
}

func TestIdempotencyTag_DistinctPerFollower(t *testing.T) {
	followerA := uuid.New()
	followerB := uuid.New()
	tagA := domain.IdempotencyTag(42, followerA)
	tagB := domain.IdempotencyTag(42, followerB)
	if tagA == tagB {
		t.Fatalf("expected distinct tags for different followers, both were %q", tagA)
	}
}

func TestIdempotencyTag_DistinctPerFill(t *testing.T) {
	followerID := uuid.New()
	tag1 := domain.IdempotencyTag(1, followerID)
	tag2 := domain.IdempotencyTag(2, followerID)
	if tag1 == tag2 {
		t.Fatalf("expected distinct tags for different master fills, both were %q", tag1)
	}
}

func TestIdempotencyTag_MaxLength20(t *testing.T) {
	tag := domain.IdempotencyTag(9223372036854775807, uuid.New())
	if len(tag) > 20 {
		t.Fatalf("tag %q is %d chars, want <= 20 (Kite tag limit)", tag, len(tag))
	}
}

func TestIdempotencyTag_AlphanumericOnly(t *testing.T) {
	tag := domain.IdempotencyTag(42, uuid.New())
	for _, r := range tag {
		isAlphaNum := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isAlphaNum {
			t.Fatalf("tag %q contains non-alphanumeric rune %q", tag, r)
		}
	}
}
