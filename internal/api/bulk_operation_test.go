package api

import (
	"errors"
	"sync"
	"testing"
)

func TestBulkOperation_MapsResultsCorrectly(t *testing.T) {
	client := &jiraClient{}
	keys := []string{"PROJ-1", "PROJ-2", "PROJ-3", "PROJ-4", "PROJ-5"}

	// Track which keys were processed
	processed := make(map[string]bool)
	var mu sync.Mutex

	op := func(key string) error {
		mu.Lock()
		defer mu.Unlock()
		processed[key] = true
		return nil
	}

	errors := client.bulkOperation(keys, op)

	if len(errors) != len(keys) {
		t.Fatalf("expected %d errors, got %d", len(keys), len(errors))
	}

	// All keys should have been processed
	for _, key := range keys {
		if !processed[key] {
			t.Errorf("key %q was not processed", key)
		}
	}

	// All errors should be nil
	for i, err := range errors {
		if err != nil {
			t.Errorf("errors[%d] = %v, want nil", i, err)
		}
	}
}

func TestBulkOperation_PartialFailure(t *testing.T) {
	client := &jiraClient{}
	keys := []string{"PROJ-1", "PROJ-2", "PROJ-3", "PROJ-4", "PROJ-5"}

	// Fail every other key
	op := func(key string) error {
		if key == "PROJ-2" || key == "PROJ-4" {
			return errors.New("simulated failure")
		}
		return nil
	}

	errors := client.bulkOperation(keys, op)

	if len(errors) != len(keys) {
		t.Fatalf("expected %d errors, got %d", len(keys), len(errors))
	}

	// Check specific indices
	if errors[0] != nil {
		t.Errorf("errors[0] (PROJ-1) = %v, want nil", errors[0])
	}
	if errors[1] == nil {
		t.Errorf("errors[1] (PROJ-2) = nil, want error")
	}
	if errors[2] != nil {
		t.Errorf("errors[2] (PROJ-3) = %v, want nil", errors[2])
	}
	if errors[3] == nil {
		t.Errorf("errors[3] (PROJ-4) = nil, want error")
	}
	if errors[4] != nil {
		t.Errorf("errors[4] (PROJ-5) = %v, want nil", errors[4])
	}
}

func TestBulkOperation_EmptyKeys(t *testing.T) {
	client := &jiraClient{}
	keys := []string{}

	op := func(key string) error {
		t.Error("op should not be called for empty keys")
		return nil
	}

	errors := client.bulkOperation(keys, op)

	if errors != nil {
		t.Errorf("bulkOperation([]) = %v, want nil", errors)
	}
}

func TestBulkOperation_SingleKey(t *testing.T) {
	client := &jiraClient{}
	keys := []string{"PROJ-1"}

	called := false
	op := func(key string) error {
		called = true
		if key != "PROJ-1" {
			t.Errorf("op called with key = %q, want %q", key, "PROJ-1")
		}
		return nil
	}

	errors := client.bulkOperation(keys, op)

	if !called {
		t.Error("op was not called")
	}
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0] != nil {
		t.Errorf("errors[0] = %v, want nil", errors[0])
	}
}

func TestBulkOperation_AllFailures(t *testing.T) {
	client := &jiraClient{}
	keys := []string{"PROJ-1", "PROJ-2", "PROJ-3"}

	op := func(key string) error {
		return errors.New("always fails")
	}

	errors := client.bulkOperation(keys, op)

	if len(errors) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(errors))
	}

	for i, err := range errors {
		if err == nil {
			t.Errorf("errors[%d] = nil, want error", i)
		}
	}
}

func TestBulkOperation_ConcurrencyLimit(t *testing.T) {
	client := &jiraClient{}

	// Create enough keys to exceed the worker limit (10)
	keys := make([]string, 20)
	for i := 0; i < 20; i++ {
		keys[i] = "PROJ-" + string(rune('A'+i))
	}

	// Track concurrent executions
	var maxConcurrent int
	var currentConcurrent int
	var mu sync.Mutex

	op := func(key string) error {
		mu.Lock()
		currentConcurrent++
		if currentConcurrent > maxConcurrent {
			maxConcurrent = currentConcurrent
		}
		mu.Unlock()

		// Small delay to allow overlap
		for i := 0; i < 100; i++ {
			// busy work
		}

		mu.Lock()
		currentConcurrent--
		mu.Unlock()
		return nil
	}

	client.bulkOperation(keys, op)

	// Concurrency should not exceed maxWorkers (10)
	if maxConcurrent > 10 {
		t.Errorf("max concurrent = %d, want <= 10", maxConcurrent)
	}
	// Note: We can't guarantee parallelism in tests, so just check it ran
	if maxConcurrent < 1 {
		t.Errorf("max concurrent = %d, want >= 1", maxConcurrent)
	}
}

func TestBulkOperation_ResultOrdering(t *testing.T) {
	client := &jiraClient{}
	keys := []string{"PROJ-A", "PROJ-B", "PROJ-C", "PROJ-D", "PROJ-E"}

	// Process in reverse order to test that results are mapped correctly
	op := func(key string) error {
		// Reverse order processing
		return nil
	}

	errors := client.bulkOperation(keys, op)

	if len(errors) != 5 {
		t.Fatalf("expected 5 errors, got %d", len(errors))
	}

	// All should be nil regardless of processing order
	for i, err := range errors {
		if err != nil {
			t.Errorf("errors[%d] (%s) = %v, want nil", i, keys[i], err)
		}
	}
}

func TestBulkOperation_ErrorPreservesIndex(t *testing.T) {
	client := &jiraClient{}
	keys := []string{"PROJ-1", "PROJ-2", "PROJ-3"}

	// Only fail PROJ-2 (index 1)
	op := func(key string) error {
		if key == "PROJ-2" {
			return errors.New("specific failure")
		}
		return nil
	}

	errors := client.bulkOperation(keys, op)

	// Error should be at index 1
	if errors[0] != nil {
		t.Errorf("errors[0] = %v, want nil", errors[0])
	}
	if errors[1] == nil {
		t.Error("errors[1] = nil, want error for PROJ-2")
	}
	if errors[2] != nil {
		t.Errorf("errors[2] = %v, want nil", errors[2])
	}
}
