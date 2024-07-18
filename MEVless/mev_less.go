package MEVless

import (
	"github.com/yu-org/yu/common"
	"github.com/yu-org/yu/core/tripod"
	"github.com/yu-org/yu/core/types"
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
	return m.Pool.Pack(numLimit)
}

func (m *MEVless) PackFor(blockNum common.BlockNum, numLimit uint64, filter func(*types.SignedTxn) bool) ([]*types.SignedTxn, error) {
	err := m.OrderCommitment(blockNum)
	if err != nil {
		return nil, err
	}
	return m.Pool.PackFor(numLimit, filter)
}

func (m *MEVless) OrderCommitment(blockNum common.BlockNum) error {
	txns, err := m.Pool.PackFor(m.cfg.PackNumber, func(txn *types.SignedTxn) bool {
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

	sequence := m.sortTxns(txns)

	m.Pool.SetOrder(sequence)

	// send event to client to let them know the tx order commitment
	m.orderCommitments <- &OrderCommitment{
		BlockNumber: blockNum,
		Sequence:    sequence,
	}

	// TODO: sync the order commitment to other P2P nodes

	// sleep for a while so that clients can send their tx-content onchain.
	time.Sleep(800 * time.Millisecond)

	return m.Pool.Reset(txns)
}

func (m *MEVless) sortTxns(txns []*types.SignedTxn) map[int]common.Hash {
	order := make(map[int]common.Hash)
	sort.Slice(txns, func(i, j int) bool {
		return txns[i].GetTips() > txns[j].GetTips()
	})
	for i, txn := range txns {
		hashStr := strings.TrimPrefix(txn.GetParams(), Prefix)
		order[i] = common.BytesToHash([]byte(hashStr))
	}
	return order
}

func (m *MEVless) VerifyBlock(block *types.Block) error {
	// TODO: verify tx order commitment from other miner node
	return nil
}

type OrderCommitment struct {
	BlockNumber common.BlockNum     `json:"block_number"`
	Sequence    map[int]common.Hash `json:"sequence"`
}
