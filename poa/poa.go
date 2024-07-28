package poa

import (
	"bytes"
	"fmt"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/yu-org/nine-tripods/MEVless"
	. "github.com/yu-org/yu/common"
	"github.com/yu-org/yu/common/yerror"
	. "github.com/yu-org/yu/core/keypair"
	. "github.com/yu-org/yu/core/tripod"
	. "github.com/yu-org/yu/core/types"
	"github.com/yu-org/yu/utils/log"
	"go.uber.org/atomic"
	"time"
)

type Poa struct {
	*Tripod

	MevLess *MEVless.MEVless `tripod:"mevless,omitempty"`

	// key: crypto address, generate from pubkey
	validatorsMap map[Address]peer.ID
	myPubkey      PubKey
	myPrivKey     PrivKey

	validatorsList []Address

	currentHeight *atomic.Uint32

	blockInterval int
	packNum       uint64
	recvChan      chan *Block
	// local node index in addrs
	nodeIdx int
}

type ValidatorInfo struct {
	Pubkey PubKey
	P2pID  peer.ID
}

func NewPoa(cfg *PoaConfig) *Poa {
	pub, priv, infos, err := resolveConfig(cfg)
	if err != nil {
		logrus.Fatal("resolve poa config error: ", err)
	}
	return newPoa(pub, priv, infos, cfg.BlockInterval, cfg.PackNum)
}

func newPoa(myPubkey PubKey, myPrivkey PrivKey, addrIps []ValidatorInfo, interval int, packNum uint64) *Poa {
	tri := NewTripod()

	var nodeIdx int

	validatorsAddr := make([]Address, 0)
	validators := make(map[Address]peer.ID)
	for _, addrIp := range addrIps {
		addr := addrIp.Pubkey.Address()
		validators[addr] = addrIp.P2pID

		if addr == myPubkey.Address() {
			nodeIdx = len(validatorsAddr)
		}

		validatorsAddr = append(validatorsAddr, addr)
	}

	p := &Poa{
		Tripod:         tri,
		validatorsMap:  validators,
		validatorsList: validatorsAddr,
		myPubkey:       myPubkey,
		myPrivKey:      myPrivkey,
		currentHeight:  atomic.NewUint32(0),
		blockInterval:  interval,
		packNum:        packNum,
		recvChan:       make(chan *Block, 10),
		nodeIdx:        nodeIdx,
	}
	//p.SetInit(p)
	//p.SetTxnChecker(p)
	//p.SetBlockCycle(p)
	//p.SetBlockVerifier(p)
	return p
}

func (h *Poa) ValidatorsP2pID() (peers []peer.ID) {
	for _, id := range h.validatorsMap {
		peers = append(peers, id)
	}
	return
}

func (h *Poa) LocalAddress() Address {
	return h.myPubkey.Address()
}

func (h *Poa) CheckTxn(txn *SignedTxn) error {
	// return metamask.CheckMetamaskSig(txn)
	return nil
}

func (h *Poa) VerifyBlock(block *Block) error {
	minerPubkey, err := PubKeyFromBytes(block.MinerPubkey)
	if err != nil {
		logrus.Warnf("parse pubkey(%s) error: %v", block.MinerPubkey, err)
		return err
	}
	if _, ok := h.validatorsMap[minerPubkey.Address()]; !ok {
		logrus.Warn("illegal miner: ", minerPubkey.StringWithType())
		return errors.Errorf("miner(%s) is not validator", minerPubkey.Address())
	}
	if !minerPubkey.VerifySignature(block.Hash.Bytes(), block.MinerSignature) {
		return yerror.BlockSignatureIllegal(block.Hash)
	}
	return nil
}

func (h *Poa) InitChain(block *Block) {

	go func() {
		for {
			msg, err := h.P2pNetwork.SubP2P(StartBlockTopic)
			if err != nil {
				logrus.Error("subscribe message from P2P error: ", err)
				continue
			}
			p2pBlock, err := DecodeBlock(msg)
			if err != nil {
				logrus.Error("decode p2pBlock from p2p error: ", err)
				continue
			}
			if bytes.Equal(p2pBlock.MinerPubkey, h.myPubkey.BytesWithType()) {
				continue
			}

			logrus.Debugf("accept block(%s), height(%d), miner(%s)",
				p2pBlock.Hash.String(), p2pBlock.Height, ToHex(p2pBlock.MinerPubkey))

			if h.getCurrentHeight() > p2pBlock.Height {
				continue
			}

			err = h.RangeList(func(tri *Tripod) error {
				return tri.BlockVerifier.VerifyBlock(block)
			})
			if err != nil {
				logrus.Warnf("p2pBlock(%s) verify failed: %s", p2pBlock.Hash, err)
				continue
			}

			h.recvChan <- p2pBlock
		}
	}()
}

func (h *Poa) StartBlock(block *Block) {
	now := time.Now()
	defer func() {
		duration := time.Since(now)
		time.Sleep(time.Duration(h.blockInterval)*time.Second - duration)
	}()

	h.setCurrentHeight(block.Height)

	log.StarConsole.Info(fmt.Sprintf("start a new block, height=%d", block.Height))

	if !h.AmILeader(block.Height) {
		if h.useP2pOrSkip(block) {
			logrus.Infof("--------USE P2P Height(%d) block(%s) miner(%s)",
				block.Height, block.Hash.String(), ToHex(block.MinerPubkey))
			return
		}
	}

	logrus.Infof(" I am Leader! I mine the block for height (%d)! ", block.Height)
	var (
		txns []*SignedTxn
		err  error
	)

	if h.MevLess != nil {
		txns, err = h.MevLess.Pack(block.Height, h.packNum)
	} else {
		txns, err = h.Pool.Pack(h.packNum)
	}

	if err != nil {
		logrus.Panic("pack txns from pool: ", err)
	}

	// logrus.Info("---- the num of pack txns is ", len(txns))

	txnRoot, err := MakeTxnRoot(txns)
	if err != nil {
		logrus.Panic("make txn-root failed: ", err)
	}
	block.TxnRoot = txnRoot

	byt, _ := block.Encode()
	block.Hash = BytesToHash(Sha256(byt))

	// miner signs block
	block.MinerSignature, err = h.myPrivKey.SignData(block.Hash.Bytes())
	if err != nil {
		logrus.Panic("sign block failed: ", err)
	}
	block.MinerPubkey = h.myPubkey.BytesWithType()

	block.SetTxns(txns)

	h.State.StartBlock(block.Hash)

	blockByt, err := block.Encode()
	if err != nil {
		logrus.Panic("encode raw-block failed: ", err)
	}

	err = h.P2pNetwork.PubP2P(StartBlockTopic, blockByt)
	if err != nil {
		logrus.Panic("publish block to p2p failed: ", err)
	}
}

func (h *Poa) EndBlock(block *Block) {
	chain := h.Chain

	err := h.Execute(block)
	if err != nil {
		logrus.Panic("execute block failed: ", err)
	}

	// TODO: sync the state (execute receipt) with other nodes

	err = chain.AppendBlock(block)
	if err != nil {
		logrus.Panic("append block failed: ", err)
	}

	err = h.Pool.Reset(block.Txns)
	if err != nil {
		logrus.Panic("reset pool failed: ", err)
	}

	// log.PlusLog().Info(fmt.Sprintf("append block, height=%d, hash=%s", block.Height, block.Hash.String()))

	//logrus.WithField("block-height", block.Height).WithField("block-hash", block.Hash.String()).
	//	Info("append block")

	h.State.FinalizeBlock(block.Hash)
}

func (h *Poa) FinalizeBlock(block *Block) {
	//logrus.WithField("block-height", block.Height).WithField("block-hash", block.Hash.String()).
	//	Info("finalize block")

	log.DoubleLineConsole.Info(fmt.Sprintf("finalize block, height=%d, hash=%s", block.Height, block.Hash.String()))
	h.Chain.Finalize(block.Hash)
}

func (h *Poa) CompeteLeader(blockHeight BlockNum) Address {
	idx := (int(blockHeight) - 1) % len(h.validatorsList)
	leader := h.validatorsList[idx]
	logrus.Debugf("compete a leader(%s) in round(%d)", leader.String(), blockHeight)
	return leader
}

func (h *Poa) AmILeader(blockHeight BlockNum) bool {
	return h.CompeteLeader(blockHeight) == h.LocalAddress()
}

func (h *Poa) IsValidator(addr Address) bool {
	_, ok := h.validatorsMap[addr]
	return ok
}

func (h *Poa) useP2pOrSkip(localBlock *Block) bool {
LOOP:
	select {
	case p2pBlock := <-h.recvChan:
		if h.getCurrentHeight() > p2pBlock.Height {
			goto LOOP
		}
		localBlock.CopyFrom(p2pBlock)
		h.State.StartBlock(localBlock.Hash)
		return true
	case <-time.NewTicker(h.calulateWaitTime(localBlock)).C:
		return false
	}
}

func (h *Poa) calulateWaitTime(block *Block) time.Duration {
	height := int(block.Height)
	shouldLeaderIdx := (height - 1) % len(h.validatorsList)
	n := shouldLeaderIdx - h.nodeIdx
	if n < 0 {
		n = -n
	}

	return time.Duration(h.blockInterval+n) * time.Second
}

func (h *Poa) getCurrentHeight() BlockNum {
	return BlockNum(h.currentHeight.Load())
}

func (h *Poa) setCurrentHeight(height BlockNum) {
	h.currentHeight.Store(uint32(height))
}
