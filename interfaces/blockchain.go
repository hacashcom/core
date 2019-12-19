package interfaces

import "github.com/hacash/core/stores"

type BlockChain interface {
	InsertBlock(Block) error
	State() ChainStateOperation
	ValidateTransaction(Transaction) error
	CreateNextBlockByValidateTxs([]Transaction) (Block, uint32, error)

	SubscribeValidatedBlockOnInsert(chan Block)
	SubscribeDiamondOnCreate(chan *stores.DiamondSmelt)

	ReadPrev288BlockTimestamp(blockHeight uint64) (uint64, error)
}