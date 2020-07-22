// +build integration

package integration_test

import (
	"math/big"
	"testing"

	bep3Com "github.com/binance-chain/bep3-deputy/common"
	"github.com/binance-chain/bep3-deputy/util"
	"github.com/stretchr/testify/assert"

	"github.com/binance-chain/bep3-deputy/integration_test/common"
)

func TestConcurrentBnbToKavaSwaps(t *testing.T) {

	// 1) setup executors

	config := util.ParseConfigFromFile("deputy/config.json")

	var senderExecutors []bep3Com.Executor
	for i := range common.BnbUserMnemonics {
		senderExecutors = append(senderExecutors, setupUserExecutorBnb(*config.BnbConfig, common.BnbUserMnemonics[i]))

	}
	senderAddrs := common.BnbUserAddrs

	var receiverExecutors []bep3Com.Executor
	for i := range common.KavaUserMnemonics {
		receiverExecutors = append(receiverExecutors, setupUserExecutorKava(*config.KavaConfig, common.KavaUserMnemonics[i]))
	}
	receiverAddrs := common.KavaUserAddrs

	// 2) Send swaps from bnb to kava

	swapAmount := big.NewInt(100_000_000)
	type result struct {
		id  int
		err error
	}
	results := make(chan result)
	for i := range senderExecutors {
		go func(i int) {
			t.Logf("sending swap %d\n", i)
			err := sendCompleteSwap(t, senderExecutors[i], receiverExecutors[i], senderAddrs[i], receiverAddrs[i], swapAmount, common.BnbDeputyAddr, common.KavaDeputyAddr, 20000)
			results <- result{i, err}
		}(i)
	}

	// 3) Check results

	for range senderExecutors {
		r := <-results
		t.Logf("swap %d done, err: %v\n", r.id, r.err)
		assert.NoErrorf(t, r.err, "swap %d returned error", r.id)
	}
}
