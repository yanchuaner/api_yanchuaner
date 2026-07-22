/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletFundingWritesReserveSettlementAndRefundLedger(t *testing.T) {
	truncate(t)
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	seedUser(t, 1001, 100)

	settled := &WalletFunding{userId: 1001, tokenId: 2001, requestId: "ledger-settle"}
	require.NoError(t, settled.PreConsume(30))
	require.NoError(t, settled.Settle(-10))

	refunded := &WalletFunding{userId: 1001, tokenId: 2001, requestId: "ledger-refund"}
	require.NoError(t, refunded.PreConsume(25))
	require.NoError(t, refunded.Refund())
	// A retry must resolve through the same idempotency key and must not
	// append another refund or increase the balance a second time.
	require.NoError(t, refunded.Refund())

	var user model.User
	require.NoError(t, model.DB.First(&user, 1001).Error)
	assert.Equal(t, 80, user.Quota)

	var entries []model.QuotaLedgerEntry
	require.NoError(t, model.DB.Where("user_id = ?", 1001).Order("id asc").Find(&entries).Error)
	require.Len(t, entries, 4)
	assert.Equal(t, []int{-30, 10, -25, 25}, []int{
		entries[0].Amount,
		entries[1].Amount,
		entries[2].Amount,
		entries[3].Amount,
	})
	assert.Equal(t, model.QuotaLedgerTypeReserve, entries[0].EntryType)
	assert.Equal(t, model.QuotaLedgerTypeSettlement, entries[1].EntryType)
	assert.Equal(t, model.QuotaLedgerTypeRefund, entries[3].EntryType)
	assert.Equal(t, "ledger-refund", entries[2].RequestId)
	assert.Equal(t, "ledger-refund", entries[3].RequestId)
}
