// +build integration

package integration_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/binance-chain/go-sdk/common/types"
	ec "github.com/ethereum/go-ethereum/common"
	"github.com/kava-labs/go-sdk/client"
	"github.com/kava-labs/go-sdk/kava/bep3"
	"github.com/stretchr/testify/require"

	bep3Com "github.com/binance-chain/bep3-deputy/common"
	bnbExe "github.com/binance-chain/bep3-deputy/executor/bnb"
	kavaExe "github.com/binance-chain/bep3-deputy/executor/kava"
	"github.com/binance-chain/bep3-deputy/integration_test/common"
	"github.com/binance-chain/bep3-deputy/store"
	"github.com/binance-chain/bep3-deputy/util"
)

type logger interface {
	Log(args ...interface{})
}

func sendCompleteSwap(logger logger, senderExecutor, receiverExecutor bep3Com.Executor, senderAddr, receiverAddr string, swapAmount *big.Int, senderChainDeputyAddr, receiverChainDeputyAddr string, heightSpan int64) error {

	// 1) Send initial swap

	rndNum, err := bep3.GenerateSecureRandomNumber()
	if err != nil {
		return fmt.Errorf("couldn't generate random number: %w", err)
	}
	timestamp := time.Now().Unix()
	rndHash := ec.BytesToHash(bep3.CalculateRandomHash(rndNum, timestamp))
	htltTxHash, cmnErr := senderExecutor.HTLT(
		rndHash,
		timestamp,
		heightSpan,
		senderChainDeputyAddr,
		receiverChainDeputyAddr,
		receiverAddr,
		swapAmount,
	)
	if cmnErr != nil {
		return fmt.Errorf("couldn't send htlt tx: %w", cmnErr)
	}

	err = common.Wait(8*time.Second, func() (bool, error) {
		s := senderExecutor.GetSentTxStatus(htltTxHash)
		return s == store.TxSentStatusSuccess, nil
	})
	if err != nil {
		return fmt.Errorf("couldn't submit htlt: %w", err)
	}
	logger.Log("sender htlt created")

	// 2) Wait until deputy relays swap to receiver chain

	receiverSwapIDBz, err := receiverExecutor.CalcSwapId(rndHash, receiverChainDeputyAddr, senderAddr)
	if err != nil {
		return fmt.Errorf("couldn't calculate swap id: %w", err)
	}
	receiverSwapID := ec.BytesToHash(receiverSwapIDBz)

	err = common.Wait(60*time.Second, func() (bool, error) {
		return receiverExecutor.HasSwap(receiverSwapID)
	})
	if err != nil {
		return fmt.Errorf("deputy did not relay swap: %w", err)
	}

	logger.Log("swap created on receiver by deputy")

	// 3) Send claim on receiver

	claimTxHash, cmnErr := receiverExecutor.Claim(receiverSwapID, ec.BytesToHash(rndNum))
	if cmnErr != nil {
		return fmt.Errorf("claim couldn't be submitted: %w", cmnErr)
	}

	err = common.Wait(8*time.Second, func() (bool, error) {
		return receiverExecutor.GetSentTxStatus(claimTxHash) == store.TxSentStatusSuccess, nil
	})
	if err != nil {
		return fmt.Errorf("claim was not submitted: %w", err)
	}

	logger.Log("receiver htlt claimed")

	// 4) Wait until deputy relays claim to sender chian

	senderSwapIDBz, err := senderExecutor.CalcSwapId(rndHash, senderAddr, receiverChainDeputyAddr)
	if err != nil {
		return fmt.Errorf("couldn't calculate swap id: %w", err)
	}
	senderSwapID := ec.BytesToHash(senderSwapIDBz)

	common.Wait(10*time.Second, func() (bool, error) {
		// check the deputy has relayed the claim by checking the status of the swap
		// once claimed it is no longer claimable, if it timesout it will become refundable
		claimable, err := senderExecutor.Claimable(senderSwapID)
		if err != nil {
			return false, err
		}
		refundable, err := senderExecutor.Refundable(senderSwapID)
		if err != nil {
			return false, err
		}
		return !(claimable || refundable), nil
	})
	logger.Log("sender htlt claimed by deputy")

	return nil
}
func TestBnbToKavaSwap(t *testing.T) {

	// 1) setup executors

	config := util.ParseConfigFromFile("deputy/config.json")

	senderExecutor := setupUserExecutorBnb(*config.BnbConfig, common.BnbUserMnemonics[0])
	senderAddr := common.BnbUserAddrs[0]

	receiverExecutor := setupUserExecutorKava(*config.KavaConfig, common.KavaUserMnemonics[0])
	receiverAddr := common.KavaUserAddrs[0]

	// 2) Cache account balances

	senderBalance, err := senderExecutor.GetBalance(senderAddr)
	require.NoError(t, err)
	receiverBalance, err := receiverExecutor.GetBalance(receiverAddr)
	require.NoError(t, err)

	// 3) Send swap

	swapAmount := big.NewInt(100_000_000)
	err = sendCompleteSwap(t, senderExecutor, receiverExecutor, senderAddr, receiverAddr, swapAmount, common.BnbDeputyAddr, common.KavaDeputyAddr, 20000)
	require.NoError(t, err)

	// 4) Check balances

	senderBalanceFinal, err := senderExecutor.GetBalance(senderAddr)
	require.NoError(t, err)

	expectedSenderBalance := new(big.Int)
	expectedSenderBalance.Sub(senderBalance, swapAmount).Sub(expectedSenderBalance, big.NewInt(common.BnbHTLTFee))
	require.Zerof(t,
		expectedSenderBalance.Cmp(senderBalanceFinal),
		"expected: %s, actual: %s", expectedSenderBalance, senderBalanceFinal,
	)

	receiverBalanceFinal, err := receiverExecutor.GetBalance(receiverAddr)
	require.NoError(t, err)

	swapAmountReceiver := new(big.Int)
	swapAmountReceiver.Sub(swapAmount, config.ChainConfig.BnbFixedFee)
	expectedReceiverBalance := new(big.Int).Add(receiverBalance, swapAmountReceiver)
	require.Zero(t, config.ChainConfig.BnbRatio.Cmp(big.NewFloat(1)), "test does not support ratio conversions other than 1")
	require.Zerof(t,
		expectedReceiverBalance.Cmp(receiverBalanceFinal),
		"expected: %s, actual: %s", expectedReceiverBalance, receiverBalanceFinal,
	)

}

func TestKavaToBnbSwap(t *testing.T) {

	// 1) setup executors

	config := util.ParseConfigFromFile("deputy/config.json")

	senderExecutor := setupUserExecutorKava(*config.KavaConfig, common.KavaUserMnemonics[0])
	senderAddr := common.KavaUserAddrs[0]

	receiverExecutor := setupUserExecutorBnb(*config.BnbConfig, common.BnbUserMnemonics[0])
	receiverAddr := common.BnbUserAddrs[0]

	// 2) Cache account balances

	receiverBalance, err := receiverExecutor.GetBalance(receiverAddr)
	require.NoError(t, err)
	senderBalance, err := senderExecutor.GetBalance(senderAddr)
	require.NoError(t, err)

	// 3) Send swap

	swapAmount := big.NewInt(99_000_000)
	err = sendCompleteSwap(t, senderExecutor, receiverExecutor, senderAddr, receiverAddr, swapAmount, kavaDeputyAddr, bnbDeputyAddr, 250)
	require.NoError(t, err)

	// 4) Check balances

	senderBalanceFinal, err := senderExecutor.GetBalance(senderAddr)
	require.NoError(t, err)

	expectedSenderBalance := new(big.Int)
	expectedSenderBalance.Sub(senderBalance, swapAmount) // no bnb tx fee when sending from kava
	require.Zerof(t,
		expectedSenderBalance.Cmp(senderBalanceFinal),
		"expected: %s, actual: %s", expectedSenderBalance, senderBalanceFinal,
	)

	receiverBalanceFinal, err := receiverExecutor.GetBalance(receiverAddr)
	require.NoError(t, err)

	swapAmountReceiver := new(big.Int)
	swapAmountReceiver.Sub(swapAmount, config.ChainConfig.OtherChainFixedFee)
	expectedReceiverBalance := new(big.Int).Add(receiverBalance, swapAmountReceiver)
	expectedReceiverBalance.Sub(expectedReceiverBalance, big.NewInt(common.BnbClaimFee))
	require.Zero(t, config.ChainConfig.OtherChainRatio.Cmp(big.NewFloat(1)), "test does not support ratio conversions other than 1")
	require.Zerof(t,
		expectedReceiverBalance.Cmp(receiverBalanceFinal),
		"expected: %s, actual: %s", expectedReceiverBalance, receiverBalanceFinal,
	)
}

func setupUserExecutorBnb(bnbConfig util.BnbConfig, mnemonic string) *bnbExe.Executor {
	bnbConfig.RpcAddr = common.BnbNodeURL
	bnbConfig.Mnemonic = mnemonic
	bnbConfig.DeputyAddr = common.BnbAddressFromMnemonic(mnemonic) // not the actual deputy address
	return bnbExe.NewExecutor(types.ProdNetwork, &bnbConfig)
}

func setupUserExecutorKava(kavaConfig util.KavaConfig, mnemonic string) *kavaExe.Executor {
	kavaConfig.RpcAddr = common.KavaNodeURL
	kavaConfig.Mnemonic = mnemonic
	kavaConfig.DeputyAddr = common.KavaAddressFromMnemonic(mnemonic) // not the actual deputy address
	return kavaExe.NewExecutor(client.LocalNetwork, &kavaConfig)
}
