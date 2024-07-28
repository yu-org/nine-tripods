package MEVless

import (
	"github.com/sirupsen/logrus"
	"github.com/yu-org/yu/common"
	"github.com/yu-org/yu/core/tripod"
	"github.com/yu-org/yu/core/types"
	"slices"
	"sort"
	"strings"
	"time"
)

type MEVless struct {
	*tripod.Tripod
	cfg *Config

	orderCommitments chan *OrderCommitment
}

const Prefix = "MEVless_"

func NewMEVless(cfg *Config) *MEVless {
	tri := &MEVless{
		Tripod:           tripod.NewTripod(),
		cfg:              cfg,
		orderCommitments: make(chan *OrderCommitment),
	}
	go tri.HandleSubscribe()
	return tri
}

func (m *MEVless) Pack(blockNum common.BlockNum, numLimit uint64) ([]*types.SignedTxn, error) {
	err := m.OrderCommitment(blockNum)
	if err != nil {
		return nil, err
	}
	return m.Pool.PackFor(numLimit, func(txn *types.SignedTxn) bool {
		return txn.ParamsIsJson()
	})
}

func (m *MEVless) PackFor(blockNum common.BlockNum, numLimit uint64, filter func(*types.SignedTxn) bool) ([]*types.SignedTxn, error) {
	err := m.OrderCommitment(blockNum)
	if err != nil {
		return nil, err
	}
	return m.Pool.PackFor(numLimit, func(txn *types.SignedTxn) bool {
		if txn.ParamsIsJson() {
			return filter(txn)
		}
		return false
	})
}

func (m *MEVless) OrderCommitment(blockNum common.BlockNum) error {
	hashTxns, err := m.Pool.PackFor(m.cfg.PackNumber, func(txn *types.SignedTxn) bool {
		if txn.ParamsIsJson() {
			return false
		}
		paramStr := txn.GetParams()
		if !strings.HasPrefix(paramStr, Prefix) {
			return false
		}
		hashStr := strings.TrimPrefix(paramStr, Prefix)
		hashByt := []byte(hashStr)
		return len(hashByt) == common.HashLen
	})
	if err != nil {
		return err
	}

	sequence := m.makeOrder(hashTxns)

	// send event to client to let them know the tx order commitment
	m.orderCommitments <- &OrderCommitment{
		BlockNumber: blockNum,
		Sequence:    sequence,
	}

	// TODO: sync the order commitment to other P2P nodes

	// sleep for a while so that clients can send their tx-content onchain.
	time.Sleep(800 * time.Millisecond)

	m.Pool.Reset(hashTxns)

	m.Pool.SortTxns(func(txs []*types.SignedTxn) []*types.SignedTxn {
		sorted := make([]*types.SignedTxn, len(sequence))
		for num, hash := range sequence {
			txn, err := m.Pool.GetTxn(hash)
			if err != nil {
				logrus.Error("MEVless get txn from txpool failed: ", err)
				continue
			}
			if txn != nil {
				sorted[num] = txn
				txs = slices.DeleteFunc(txs, func(txn *types.SignedTxn) bool {
					return txn.TxnHash == hash
				})
			}
		}

		sorted = append(sorted, txs...)

		return sorted
	})

	return nil
}

func (m *MEVless) makeOrder(hashTxns []*types.SignedTxn) map[int]common.Hash {
	order := make(map[int]common.Hash)
	sort.Slice(hashTxns, func(i, j int) bool {
		return hashTxns[i].GetTips() > hashTxns[j].GetTips()
	})
	for i, txn := range hashTxns {
		hashStr := strings.TrimPrefix(txn.GetParams(), Prefix)
		order[i] = common.BytesToHash([]byte(hashStr))
	}
	return order
}

func (m *MEVless) VerifyBlock(block *types.Block) error {
	// TODO: verify tx order commitment from other miner node
	// TODO: for double-check, fetch txs from DA layers if the block does not have commitment txs.
	return nil
}

func (m *MEVless) Charge() uint64 {
	return m.cfg.Charge
}

type OrderCommitment struct {
	BlockNumber common.BlockNum     `json:"block_number"`
	Sequence    map[int]common.Hash `json:"sequence"`
}
