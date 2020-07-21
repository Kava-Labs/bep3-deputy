// +build integration

package kava

import (
	"math/big"
	"testing"
	"time"

	ec "github.com/ethereum/go-ethereum/common"
	"github.com/kava-labs/go-sdk/client"
	"github.com/kava-labs/go-sdk/kava/bep3"
	"github.com/stretchr/testify/require"

	"github.com/binance-chain/bep3-deputy/integration_test/common"
	"github.com/binance-chain/bep3-deputy/store"
	"github.com/binance-chain/bep3-deputy/util"
)

func TestSendHTLT(t *testing.T) {

	config := util.ParseConfigFromFile("../../integration_test/deputy/config.json")
	config.KavaConfig.RpcAddr = common.KavaNodeURL

	exe := NewExecutor(client.LocalNetwork, config.KavaConfig)

	// calculate swap details
	rndNum, err := bep3.GenerateSecureRandomNumber()
	require.NoError(t, err)
	timestamp := time.Now().Unix()
	rndHash := ec.BytesToHash(bep3.CalculateRandomHash(rndNum, timestamp))

	// ensure swap does not exist
	swapIDBz, err := exe.CalcSwapId(rndHash, exe.GetDeputyAddress(), common.BnbUserAddrs[0])
	require.NoError(t, err)
	swapID := ec.BytesToHash(swapIDBz)

	// send swap
	txHash, cmnErr := exe.HTLT(
		rndHash,
		timestamp,
		250,
		common.KavaUserAddrs[0], // receiver
		common.BnbUserAddrs[0],  // bnb chain sender
		common.BnbDeputyAddr,    // bnb chain receiver
		big.NewInt(100_000_000),
	)
	t.Log("error from sending swap: ", cmnErr)
	require.Nil(t, cmnErr) // require.NoError will incorrectly throw an error if cmnErr is nil (because it's a nil pointer, not nil interface value)

	common.Wait(10*time.Second, func() (bool, error) {
		s := exe.GetSentTxStatus(txHash)
		return s == store.TxSentStatusSuccess, nil
	})

	// check swap has been created
	hasSwap, err := exe.HasSwap(swapID)
	require.NoError(t, err)
	require.True(t, hasSwap)
}
func TestSendAmount(t *testing.T) {

	config := util.ParseConfigFromFile("../../integration_test/deputy/config.json")
	config.KavaConfig.RpcAddr = common.KavaNodeURL

	exe := NewExecutor(client.LocalNetwork, config.KavaConfig)

	amountToSend := big.NewInt(100_000_000)
	previousBalance, err := exe.GetBalance(config.KavaConfig.ColdWalletAddr.String())

	txHash, err := exe.SendAmount(config.KavaConfig.ColdWalletAddr.String(), amountToSend)
	require.NoError(t, err)

	common.Wait(10*time.Second, func() (bool, error) {
		s := exe.GetSentTxStatus(txHash)
		return s == store.TxSentStatusSuccess, nil
	})

	// check coins have moved
	balance, err := exe.GetBalance(config.KavaConfig.ColdWalletAddr.String())
	require.NoError(t, err)
	require.Equal(t, balance, previousBalance.Add(previousBalance, amountToSend))
}
