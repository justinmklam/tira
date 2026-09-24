package api

import (
	"errors"
	"testing"

	"github.com/justinmklam/tira/internal/models"
)

// stubClient implements only the methods exercised by these tests. Embedding the
// Client interface keeps the stub small while still satisfying it.
type stubClient struct {
	Client

	epicChildren      map[string][]models.Issue
	epicChildrenErr   error
	epicChildrenCalls int
}

func (c *stubClient) GetEpicChildren(epicKey string) ([]models.Issue, error) {
	c.epicChildrenCalls++
	if c.epicChildrenErr != nil {
		return nil, c.epicChildrenErr
	}
	return c.epicChildren[epicKey], nil
}

func (c *stubClient) SetParent(_, _ string) error { return nil }

func TestCachedClient_GetEpicChildrenCachesPerEpic(t *testing.T) {
	inner := &stubClient{epicChildren: map[string][]models.Issue{
		"EPIC-1": {{Key: "PROJ-1"}},
		"EPIC-2": {{Key: "PROJ-2"}},
	}}
	client := NewCachedClient(inner)

	for range 3 {
		children, err := client.GetEpicChildren("EPIC-1")
		if err != nil {
			t.Fatalf("GetEpicChildren() error = %v", err)
		}
		if len(children) != 1 || children[0].Key != "PROJ-1" {
			t.Fatalf("children = %+v, want PROJ-1", children)
		}
	}
	if _, err := client.GetEpicChildren("EPIC-2"); err != nil {
		t.Fatalf("GetEpicChildren(EPIC-2) error = %v", err)
	}
	if inner.epicChildrenCalls != 2 {
		t.Errorf("inner calls = %d, want 2 (one per distinct epic)", inner.epicChildrenCalls)
	}
}

func TestCachedClient_GetEpicChildrenInvalidateRefetches(t *testing.T) {
	inner := &stubClient{epicChildren: map[string][]models.Issue{"EPIC-1": {{Key: "PROJ-1"}}}}
	client := NewCachedClient(inner)

	if _, err := client.GetEpicChildren("EPIC-1"); err != nil {
		t.Fatalf("GetEpicChildren() error = %v", err)
	}
	client.Invalidate()
	if _, err := client.GetEpicChildren("EPIC-1"); err != nil {
		t.Fatalf("GetEpicChildren() after Invalidate error = %v", err)
	}
	if inner.epicChildrenCalls != 2 {
		t.Errorf("inner calls = %d, want 2 after Invalidate", inner.epicChildrenCalls)
	}
}

func TestCachedClient_GetEpicChildrenDoesNotCacheErrors(t *testing.T) {
	inner := &stubClient{epicChildrenErr: errors.New("boom")}
	client := NewCachedClient(inner)

	if _, err := client.GetEpicChildren("EPIC-1"); err == nil {
		t.Fatal("GetEpicChildren() error = nil, want error")
	}
	if _, err := client.GetEpicChildren("EPIC-1"); err == nil {
		t.Fatal("GetEpicChildren() error = nil, want error on retry")
	}
	if inner.epicChildrenCalls != 2 {
		t.Errorf("inner calls = %d, want 2 (errors are not cached)", inner.epicChildrenCalls)
	}
}

func TestCachedClient_SetParentDropsEpicChildren(t *testing.T) {
	inner := &stubClient{epicChildren: map[string][]models.Issue{"EPIC-1": {{Key: "PROJ-1"}}}}
	client := NewCachedClient(inner)

	if _, err := client.GetEpicChildren("EPIC-1"); err != nil {
		t.Fatalf("GetEpicChildren() error = %v", err)
	}
	if err := client.SetParent("PROJ-1", "EPIC-2"); err != nil {
		t.Fatalf("SetParent() error = %v", err)
	}
	if _, err := client.GetEpicChildren("EPIC-1"); err != nil {
		t.Fatalf("GetEpicChildren() after SetParent error = %v", err)
	}
	if inner.epicChildrenCalls != 2 {
		t.Errorf("inner calls = %d, want 2 after reparenting", inner.epicChildrenCalls)
	}
}
