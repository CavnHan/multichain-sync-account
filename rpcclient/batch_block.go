package rpcclient

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/CavnHan/multichain-sync-account/common/bigint"
	"github.com/ethereum/go-ethereum/log"
)

var (
	ErrBatchBlockAheadOfProvider = errors.New("the BatchBlock's internal state is ahead of the provider")
)

type BatchBlock struct {
	rpcClient *WalletChainAccountClient

	latestHeader        *BlockHeader
	lastTraversedHeader *BlockHeader

	blockConfirmationDepth *big.Int
}

func NewBatchBlock(rpcClient *WalletChainAccountClient, fromHeader *BlockHeader, confDepth *big.Int) *BatchBlock {
	return &BatchBlock{
		rpcClient:              rpcClient,
		lastTraversedHeader:    fromHeader,
		blockConfirmationDepth: confDepth,
	}
}

func (f *BatchBlock) LatestHeader() *BlockHeader {
	return f.latestHeader
}

func (f *BatchBlock) LastTraversedHeader() *BlockHeader {
	return f.lastTraversedHeader
}

func (f *BatchBlock) NextHeaders(maxSize uint64) ([]BlockHeader, error) {
	//获取最新区块
	latestHeader, err := f.rpcClient.GetBlockHeader(nil)
	if err != nil {
		return nil, fmt.Errorf("unable to query latest block: %w", err)
	} else if latestHeader == nil {
		return nil, fmt.Errorf("latest header unreported")
	} else {
		f.latestHeader = latestHeader
	}
	//计算同步结束高度，即为深度
	endHeight := new(big.Int).Sub(latestHeader.Number, f.blockConfirmationDepth)
	if endHeight.Sign() < 0 {
		return nil, nil
	}
	if f.lastTraversedHeader != nil {
		//判断是否超前
		cmp := f.lastTraversedHeader.Number.Cmp(endHeight)
		if cmp == 0 {
			return nil, nil
		} else if cmp > 0 {
			return nil, ErrBatchBlockAheadOfProvider
		}
	}
	nextHeight := bigint.Zero
	if f.lastTraversedHeader != nil {
		//下一个同步起始块
		nextHeight = new(big.Int).Add(f.lastTraversedHeader.Number, bigint.One)
	}
	//现在范围
	endHeight = bigint.Clamp(nextHeight, endHeight, maxSize)
	//计算区块差，即为需要同步的区块数量
	count := new(big.Int).Sub(endHeight, nextHeight).Uint64() + 1
	headers := make([]BlockHeader, count)
	for i := uint64(0); i < count; i++ {
		height := new(big.Int).Add(nextHeight, new(big.Int).SetUint64(i))
		blockHeader, err := f.rpcClient.GetBlockHeader(height)
		if err != nil {
			log.Error("get block info fail", "err", err)
			return nil, err
		}
		headers[i] = *blockHeader
	}

	numHeaders := len(headers)
	if numHeaders == 0 {
		return nil, nil
	}
	//更新最后一个同步块
	f.lastTraversedHeader = &headers[numHeaders-1]
	return headers, nil
}
