// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package test_runner

import (
	"log"
	"sync"
)

// CleanupHandler manages cleanup functions for test resources
type CleanupHandler struct {
	cleanupFuncs []func() error
	mu           sync.Mutex
}

// AddCleanup registers a cleanup function to be executed on test completion or cancellation
func (h *CleanupHandler) AddCleanup(fn func() error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupFuncs = append(h.cleanupFuncs, fn)
}

// RunCleanup executes all registered cleanup functions in reverse order
func (h *CleanupHandler) RunCleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Execute cleanup functions in reverse order (LIFO)
	for i := len(h.cleanupFuncs) - 1; i >= 0; i-- {
		if err := h.cleanupFuncs[i](); err != nil {
			log.Printf("Cleanup error: %v", err)
		}
	}
	
	// Clear the cleanup functions after execution
	h.cleanupFuncs = nil
}