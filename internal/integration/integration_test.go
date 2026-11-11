// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"miniflux.app/v2/internal/model"
)

func TestDeliverEntriesIndividuallyNeverResendsSuccessfulEntries(t *testing.T) {
	entries := model.Entries{
		{ID: 1, URL: "https://example.org/entry1"},
		{ID: 2, URL: "https://example.org/entry2"},
		{ID: 3, URL: "https://example.org/entry3"},
	}

	attempts := map[int64]int{}
	deliverEntriesIndividually(entries, func(entry *model.Entry) error {
		attempts[entry.ID]++
		if entry.ID == 2 && attempts[entry.ID] == 1 {
			return errors.New("server returned status 500")
		}
		return nil
	})

	if attempts[1] != 1 {
		t.Errorf("entry 1 was sent %d times, expected exactly 1", attempts[1])
	}
	if attempts[3] != 1 {
		t.Errorf("entry 3 was sent %d times, expected exactly 1", attempts[3])
	}
	if attempts[2] != 2 {
		t.Errorf("entry 2 was sent %d times, expected exactly 2 (initial attempt + one retry)", attempts[2])
	}
}

func TestDeliverEntriesIndividuallyDoesNotRetryUncertainDelivery(t *testing.T) {
	entries := model.Entries{
		{ID: 1, URL: "https://example.org/entry1"},
		{ID: 2, URL: "https://example.org/entry2"},
	}

	attempts := map[int64]int{}
	deliverEntriesIndividually(entries, func(entry *model.Entry) error {
		attempts[entry.ID]++
		if entry.ID == 1 {
			return fmt.Errorf("telegram: unable to send request: %w", context.DeadlineExceeded)
		}
		return nil
	})

	if attempts[1] != 1 {
		t.Errorf("entry 1 was sent %d times, expected exactly 1 (a timed-out entry must not be resent)", attempts[1])
	}
	if attempts[2] != 1 {
		t.Errorf("entry 2 was sent %d times, expected exactly 1", attempts[2])
	}
}

func TestDeliverEntriesIndividuallyRetriesDefinitiveFailureOnlyOnce(t *testing.T) {
	entries := model.Entries{
		{ID: 1, URL: "https://example.org/entry1"},
	}

	attempts := 0
	deliverEntriesIndividually(entries, func(entry *model.Entry) error {
		attempts++
		return errors.New("server returned status 500")
	})

	if attempts != 2 {
		t.Errorf("entry was sent %d times, expected exactly 2 (initial attempt + one retry)", attempts)
	}
}

func TestIsUncertainDeliveryError(t *testing.T) {
	scenarios := []struct {
		name     string
		err      error
		expected bool
	}{
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped context deadline exceeded", fmt.Errorf("send request: %w", context.DeadlineExceeded), true},
		{"network timeout", &net.DNSError{IsTimeout: true}, true},
		{"wrapped network timeout", fmt.Errorf("send request: %w", &net.DNSError{IsTimeout: true}), true},
		{"definitive server error", errors.New("server returned status 500"), false},
		{"network error without timeout", &net.DNSError{IsTimeout: false}, false},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if result := isUncertainDeliveryError(scenario.err); result != scenario.expected {
				t.Errorf("expected %v, got %v", scenario.expected, result)
			}
		})
	}
}
