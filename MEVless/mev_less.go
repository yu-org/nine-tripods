package MEVless

import (
	"github.com/yu-org/yu/common"
	"github.com/yu-org/yu/core/tripod"
	"github.com/yu-org/yu/core/types"
	"sort"
	"strings"
)

type MEVless struct {
	*tripod.Tripod
	cfg       *Config
	orderTxns types.SignedTxns
}

const Prefix = "MEVless_"

func NewMEVless(cfg *Config) *MEVless {
	tri := &MEVless{
		Tripod:    tripod.NewTripod(),
		cfg:       cfg,
		orderTxns: make([]*types.SignedTxn, 0),
	}
	return tri
}

func (m *MEVless) Pack(numLimit uint64) ([]*types.SignedTxn, error) {
	err := m.OrderCommitment()
	if err != nil {
		return nil, err
	}
	return m.Pool.Pack(numLimit)
}

func (m *MEVless) PackFor(numLimit uint64, filter func(*types.SignedTxn) bool) ([]*types.SignedTxn, error) {
	err := m.OrderCommitment()
	if err != nil {
		return nil, err
	}
	return m.Pool.PackFor(numLimit, filter)
}

func (m *MEVless) OrderCommitment() error {
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

	m.Pool.SetOrder(m.sortTxns(txns))

	// TODO: send event to client to let them know the tx order commitment

	// TODO: sync the order commitment to other P2P nodes

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
	BlockNumber uint64              `json:"block_number"`
	Sequence    map[int]common.Hash `json:"sequence"`
}
