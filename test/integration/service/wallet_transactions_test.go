package integration

import (
	"testing"
	"time"

	"velocity/internal/persistence/postgres/generated"
	"velocity/internal/persistence/postgres/repository"
	"velocity/internal/service/walletservice"
	"velocity/test/integration"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// newWalletTestService builds a walletservice.Service against the real
// DB, plus a WalletTransactionRepository (not currently exposed on
// TestContext) needed to seed/inspect ledger rows directly.
func newWalletTestService(tc *integration.TestContext) (*walletservice.Service, repository.WalletTransactionRepository) {
	txRepo := repository.NewWalletTransactionRepository(tc.DB)
	svc := walletservice.New(tc.WalletRepo, txRepo)
	return svc, txRepo
}

func createWalletTestUser(t *testing.T, tc *integration.TestContext) int64 {
	t.Helper()

	userID := time.Now().UnixNano()

	_, err := tc.UserRepo.Create(
		tc.Ctx,
		generated.CreateUserParams{
			ID:        userID,
			Email:     uuid.NewString() + "@test.com",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	)
	require.NoError(t, err)

	return userID
}

func createWalletTestWallet(
	t *testing.T,
	tc *integration.TestContext,
	userID int64,
	asset string,
) {
	t.Helper()

	_, err := tc.WalletRepo.Create(
		tc.Ctx,
		generated.CreateWalletParams{
			UserID:    userID,
			Asset:     asset,
			Available: 0,
			Locked:    0,
		},
	)
	require.NoError(t, err)
}

// TestListTransactionsByAsset_ReturnsRecordedTransactions covers the
// core case: a deposit and a withdrawal recorded against a wallet both
// show up, most recent first, when listing that asset's transactions.
//
// NOTE: this uses DepositExternal/WithdrawExternal, not Deposit/
// Withdraw. Deposit and Withdraw - the methods actually wired to
// POST /wallets/deposit and POST /wallets/withdraw - update the wallet
// balance but never call transactionRepo.Create. Only the External
// variants (currently uncalled anywhere in the codebase) write a
// wallet_transactions row. As written today, a deposit or withdrawal
// made through the live HTTP API will NOT appear in
// GET /wallets/:asset/transactions. See TestDeposit_ViaHandlerPath_
// DoesNotAppearInLedger below, which documents this directly. You'll
// likely want either handler.Deposit/Withdraw to call the External
// service methods, or Deposit/Withdraw themselves to record a
// transaction, before relying on this endpoint in production.
func TestListTransactionsByAsset_ReturnsRecordedTransactions(t *testing.T) {
	tc := integration.NewTestContext(t)
	svc, _ := newWalletTestService(tc)

	userID := createWalletTestUser(t, tc)
	asset := "USD_" + uuid.NewString()[:8]
	createWalletTestWallet(t, tc, userID, asset)

	require.NoError(t, svc.DepositExternal(tc.Ctx, userID, asset, 500))
	require.NoError(t, svc.WithdrawExternal(tc.Ctx, userID, asset, 200))

	txs, err := svc.ListTransactionsByAsset(tc.Ctx, userID, asset)
	require.NoError(t, err)
	require.Len(t, txs, 2)

	// ORDER BY created_at DESC, id DESC - most recent (the withdrawal)
	// first.
	require.Equal(t, "WITHDRAWAL", txs[0].Type)
	require.Equal(t, int64(200), txs[0].Amount)

	require.Equal(t, "DEPOSIT", txs[1].Type)
	require.Equal(t, int64(500), txs[1].Amount)

	wallet, err := tc.WalletRepo.Get(tc.Ctx, userID, asset)
	require.NoError(t, err)
	require.Equal(t, int64(300), wallet.Available)
}

// TestListTransactionsByAsset_ScopedByAsset ensures a transaction on
// one asset never leaks into the ledger of another asset for the same
// user.
func TestListTransactionsByAsset_ScopedByAsset(t *testing.T) {
	tc := integration.NewTestContext(t)
	svc, _ := newWalletTestService(tc)

	userID := createWalletTestUser(t, tc)

	assetA := "AAA_" + uuid.NewString()[:8]
	assetB := "BBB_" + uuid.NewString()[:8]

	createWalletTestWallet(t, tc, userID, assetA)
	createWalletTestWallet(t, tc, userID, assetB)

	require.NoError(t, svc.DepositExternal(tc.Ctx, userID, assetA, 100))
	require.NoError(t, svc.DepositExternal(tc.Ctx, userID, assetB, 999))

	txsA, err := svc.ListTransactionsByAsset(tc.Ctx, userID, assetA)
	require.NoError(t, err)
	require.Len(t, txsA, 1)
	require.Equal(t, int64(100), txsA[0].Amount)
	require.Equal(t, assetA, txsA[0].Asset)
}

// TestListTransactionsByAsset_ScopedByUser ensures one user can never
// see another user's ledger entries, even for the same asset.
func TestListTransactionsByAsset_ScopedByUser(t *testing.T) {
	tc := integration.NewTestContext(t)
	svc, _ := newWalletTestService(tc)

	userA := createWalletTestUser(t, tc)
	userB := createWalletTestUser(t, tc)
	asset := "SHARED_" + uuid.NewString()[:8]

	createWalletTestWallet(t, tc, userA, asset)
	createWalletTestWallet(t, tc, userB, asset)

	require.NoError(t, svc.DepositExternal(tc.Ctx, userA, asset, 111))
	require.NoError(t, svc.DepositExternal(tc.Ctx, userB, asset, 222))

	txsA, err := svc.ListTransactionsByAsset(tc.Ctx, userA, asset)
	require.NoError(t, err)
	require.Len(t, txsA, 1)
	require.Equal(t, int64(111), txsA[0].Amount)
}

// TestListTransactionsByAsset_NoTransactions confirms a wallet with no
// activity returns an empty (not nil-panicking, not erroring) list.
func TestListTransactionsByAsset_NoTransactions(t *testing.T) {
	tc := integration.NewTestContext(t)
	svc, _ := newWalletTestService(tc)

	userID := createWalletTestUser(t, tc)
	asset := "EMPTY_" + uuid.NewString()[:8]
	createWalletTestWallet(t, tc, userID, asset)

	txs, err := svc.ListTransactionsByAsset(tc.Ctx, userID, asset)
	require.NoError(t, err)
	require.Empty(t, txs)
}

// TestDeposit_ViaHandlerPath_DoesNotAppearInLedger pins down the gap
// described above: Deposit is the method the live HTTP handler calls
// for POST /wallets/deposit, and it updates the wallet balance
// correctly, but it never writes a wallet_transactions row, so it is
// invisible to the new ledger endpoint. If/when Deposit and Withdraw
// are wired to record transactions, this test's final assertion
// (Empty) should be changed to expect one entry - that flip is the
// signal the fix landed.
func TestDeposit_ViaHandlerPath_DoesNotAppearInLedger(t *testing.T) {
	tc := integration.NewTestContext(t)
	svc, _ := newWalletTestService(tc)

	userID := createWalletTestUser(t, tc)
	asset := "GAP_" + uuid.NewString()[:8]
	createWalletTestWallet(t, tc, userID, asset)

	require.NoError(t, svc.Deposit(tc.Ctx, userID, asset, 500))

	wallet, err := tc.WalletRepo.Get(tc.Ctx, userID, asset)
	require.NoError(t, err)
	require.Equal(t, int64(500), wallet.Available)

	txs, err := svc.ListTransactionsByAsset(tc.Ctx, userID, asset)
	require.NoError(t, err)
	require.Empty(
		t,
		txs,
		"Deposit() does not record a wallet_transactions row today; "+
			"this will need to change once the HTTP deposit endpoint "+
			"is expected to show up in its own transaction history",
	)
}