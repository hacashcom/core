package blocks

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/hacash/core/fields"
	"github.com/hacash/core/interfaces"
	"github.com/hacash/core/interfacev2"
	"github.com/hacash/core/transactions"
)

type Block_v1 struct {
	// head
	/* Version   fields.VarUint1 */
	Height           fields.BlockHeight
	Timestamp        fields.BlockTxTimestamp
	PrevHash         fields.Hash
	MrklRoot         fields.Hash
	TransactionCount fields.VarUint4
	// meta
	Nonce        fields.VarUint4 // 挖矿随机值
	Difficulty   fields.VarUint4 // 目标难度值
	WitnessStage fields.VarUint2 // 见证数量级别
	// body
	Transactions []interfaces.Transaction

	/* -------- -------- */

	// cache data
	hash fields.Hash

	// mark data
	originMark string // 原产地标志: "", "miner", "discover"

	insertLock sync.RWMutex
}

func NewEmptyBlockVersion1(prevBlockHead interfaces.BlockHeadMetaRead) *Block_v1 {
	empty := NewEmptyBlockV1()
	if prevBlockHead != nil {
		empty.PrevHash = prevBlockHead.Hash()
		empty.Height = fields.BlockHeight(prevBlockHead.GetHeight() + 1)
		empty.Difficulty = fields.VarUint4(prevBlockHead.GetDifficulty())
	}
	return empty
}

func NewEmptyBlockV1() *Block_v1 {
	curt := time.Now().Unix()
	empty := &Block_v1{
		Height:           0,
		Timestamp:        fields.BlockTxTimestamp(curt),
		PrevHash:         fields.EmptyZeroBytes32,
		MrklRoot:         fields.EmptyZeroBytes32,
		TransactionCount: 0,
		Nonce:            0,
		Difficulty:       0,
		WitnessStage:     0,
		originMark:       "",
		insertLock:       sync.RWMutex{},
	}
	return empty
}

// copy
func (block *Block_v1) CopyHeadMetaForMining() interfacev2.Block {
	newblock := NewEmptyBlockVersion1(nil)
	newblock.Height = block.Height
	newblock.Timestamp = block.Timestamp
	newblock.PrevHash = append([]byte{}, block.PrevHash...)
	newblock.MrklRoot = append([]byte{}, block.MrklRoot...)
	newblock.TransactionCount = block.TransactionCount
	newblock.Nonce = block.Nonce
	newblock.Difficulty = block.Difficulty
	newblock.WitnessStage = block.WitnessStage
	// ok
	return newblock
}

func (block *Block_v1) CopyHeadMetaForMiningV3() interfaces.Block {
	blk := block.CopyHeadMetaForMining()
	return blk.(interfaces.Block)
}

func (block *Block_v1) CopyForMiningV3() interfaces.Block {
	blk := block.CopyForMining()
	return blk.(interfaces.Block)
}

// copy
func (block *Block_v1) CopyForMining() interfacev2.Block {
	// copy head and meta
	newblock := block.CopyHeadMetaForMining()
	// copy coinbase and txs
	trs := block.GetTransactions()
	newtrs := trs
	if len(trs) > 0 {
		newtrs = append([]interfacev2.Transaction{}, trs[0].Copy())
		newtrs = append(newtrs, trs[1:]...)
	}
	newblock.SetTransactions(newtrs)
	// ok
	return newblock
}

// origin: "sync", "discover", "mining"
func (block *Block_v1) OriginMark() string {
	if block.originMark == "" {
		return "sync"
	}
	return block.originMark
}
func (block *Block_v1) SetOriginMark(mark string) {
	block.originMark = mark
}

func (block *Block_v1) Version() uint8 {
	return 1
}

func (block *Block_v1) SerializeHead() ([]byte, error) {
	var buffer = new(bytes.Buffer)
	buffer.Write([]byte{block.Version()})
	b2, _ := block.Height.Serialize()
	b3, _ := block.Timestamp.Serialize()
	b4, _ := block.PrevHash.Serialize()
	b5, _ := block.MrklRoot.Serialize()
	b6, _ := block.TransactionCount.Serialize()
	buffer.Write(b2)
	buffer.Write(b3)
	buffer.Write(b4)
	buffer.Write(b5)
	buffer.Write(b6)
	return buffer.Bytes(), nil
}

func (block *Block_v1) SerializeBody() ([]byte, error) {

	var buffer = new(bytes.Buffer)
	b1, e1 := block.SerializeMeta()
	if e1 != nil {
		return nil, e1
	}
	b2, e2 := block.SerializeTransactions(nil)
	if e2 != nil {
		return nil, e2
	}
	buffer.Write(b1)
	buffer.Write(b2)
	return buffer.Bytes(), nil

}

func (block *Block_v1) SerializeMeta() ([]byte, error) {
	var buffer = new(bytes.Buffer)
	b1, _ := block.Nonce.Serialize() // miner nonce
	b2, _ := block.Difficulty.Serialize()
	b3, _ := block.WitnessStage.Serialize()
	buffer.Write(b1)
	buffer.Write(b2)
	buffer.Write(b3)
	return buffer.Bytes(), nil

}

func (block *Block_v1) SerializeTransactions(itr interfacev2.SerializeTransactionsIterator) ([]byte, error) {
	var buffer = new(bytes.Buffer)
	var trslen = uint32(len(block.Transactions))
	if itr != nil { // 迭代器
		itr.Init(trslen)
	}
	for i := uint32(0); i < trslen; i++ {
		var trs = block.Transactions[i]
		var bi, e = trs.Serialize()
		if e != nil {
			return nil, e
		}
		buffer.Write(bi)
		if itr != nil { // 迭代器
			itr.FinishOneTrs(i, trs.(interfacev2.Transaction), bi)
		}
	}
	return buffer.Bytes(), nil

}

func (block *Block_v1) SerializeExcludeTransactions() ([]byte, error) {
	var buffer = new(bytes.Buffer)

	head, _ := block.SerializeHead()
	buffer.Write(head)
	meta, _ := block.SerializeMeta()
	buffer.Write(meta)

	return buffer.Bytes(), nil
}

// 序列化 与 反序列化
func (block *Block_v1) Serialize() ([]byte, error) {

	var buffer = new(bytes.Buffer)

	head, _ := block.SerializeHead()
	buffer.Write(head)
	body, _ := block.SerializeBody()
	buffer.Write(body)

	return buffer.Bytes(), nil
}

func (block *Block_v1) ParseHead(buf []byte, seek uint32) (uint32, error) {
	if len(buf) < int(seek)+BlockHeadSize-1 {
		return 0, fmt.Errorf("buf length error.")
	}
	//fmt.Println(*buf)
	//fmt.Println(seek)
	//fmt.Println((*buf)[seek:])
	//m1, _ := block.Version.Parse(buf, seek)
	m2, e := block.Height.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	m3, e := block.Timestamp.Parse(buf, m2)
	if e != nil {
		return 0, e
	}
	m4, e := block.PrevHash.Parse(buf, m3)
	if e != nil {
		return 0, e
	}
	m5, e := block.MrklRoot.Parse(buf, m4)
	if e != nil {
		return 0, e
	}
	m6, e := block.TransactionCount.Parse(buf, m5)
	if e != nil {
		return 0, e
	}
	iseek := m6
	return iseek, nil
}

func (block *Block_v1) ParseMeta(buf []byte, seek uint32) (uint32, error) {
	var e error = nil
	seek, e = block.Nonce.Parse(buf, seek) // miner nonce
	if e != nil {
		return 0, e
	}
	seek, e = block.Difficulty.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = block.WitnessStage.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	return seek, nil
}

func (block *Block_v1) ParseExcludeTransactions(buf []byte, seek uint32) (uint32, error) {
	var e error = nil
	seek, e = block.ParseHead(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = block.ParseMeta(buf, seek)
	if e != nil {
		return 0, e
	}
	return seek, nil
}

func (block *Block_v1) ParseTransactions(buf []byte, seek uint32) (uint32, error) {
	length := int(block.TransactionCount)
	block.Transactions = make([]interfaces.Transaction, length)
	for i := 0; i < length; i++ {
		var trx, sk, err = transactions.ParseTransaction(buf, seek)
		if err != nil {
			return seek, err
		}
		block.Transactions[i] = trx.(interfaces.Transaction)
		seek = sk
	}
	return seek, nil

}

func (block *Block_v1) ParseBody(buf []byte, seek uint32) (uint32, error) {
	var e error = nil
	seek, e = block.ParseMeta(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = block.ParseTransactions(buf, seek)
	if e != nil {
		return 0, e
	}
	return seek, nil
}

func (block *Block_v1) Parse(buf []byte, seek uint32) (uint32, error) {
	// head
	iseek, e := block.ParseHead(buf, seek)
	if e != nil {
		return 0, e
	}
	iseek2, e := block.ParseBody(buf, iseek)
	if e != nil {
		return 0, e
	}
	return iseek2, nil
}

func (block *Block_v1) Size() uint32 {
	totalsize := 1 +
		// head
		block.Height.Size() +
		block.Timestamp.Size() +
		block.PrevHash.Size() +
		block.MrklRoot.Size() +
		block.TransactionCount.Size() +
		// meta
		block.Nonce.Size() +
		block.Difficulty.Size() +
		block.WitnessStage.Size()
	// trs
	for i := uint32(0); i < uint32(block.TransactionCount); i++ {
		totalsize += block.Transactions[i].Size()
	}
	return totalsize
}

// HASH
func (block *Block_v1) Hash() fields.Hash {
	block.insertLock.Lock()
	defer block.insertLock.Unlock()

	if block.hash == nil {
		block.hash = block.hashFreshUnsafe()
	}
	return block.hash
}

func (block *Block_v1) HashFresh() fields.Hash {
	block.insertLock.Lock()
	defer block.insertLock.Unlock()

	return block.hashFreshUnsafe()
}

func (block *Block_v1) hashFreshUnsafe() fields.Hash {
	block.hash = CalculateBlockHash(block)
	return block.hash
}

// 刷新所有缓存数据
func (block *Block_v1) Fresh() {
	block.hash = nil
}

func (block *Block_v1) GetHeight() uint64 {
	return uint64(block.Height)
}
func (block *Block_v1) GetTimestamp() uint64 {
	return uint64(block.Timestamp)
}
func (block *Block_v1) GetPrevHash() fields.Hash {
	return block.PrevHash
}
func (block *Block_v1) GetDifficulty() uint32 {
	return uint32(block.Difficulty)
}
func (block *Block_v1) GetWitnessStage() uint16 {
	return uint16(block.WitnessStage)
}
func (block *Block_v1) GetNonce() uint32 {
	return uint32(block.Nonce)
}
func (block *Block_v1) GetNonceByte() []byte {
	nnbts := make([]byte, 4)
	binary.BigEndian.PutUint32(nnbts, uint32(block.Nonce))
	return nnbts
}
func (block *Block_v1) GetTransactionCount() uint32 {
	return uint32(block.TransactionCount)
}
func (block *Block_v1) GetCustomerTransactionCount() uint32 {
	// drop coinbase trs
	real_count := uint32(block.TransactionCount) - 1
	return real_count
}
func (block *Block_v1) GetMrklRoot() fields.Hash {
	return block.MrklRoot
}
func (block *Block_v1) SetMrklRoot(root fields.Hash) {
	block.MrklRoot = root
}
func (block *Block_v1) SetNonce(n uint32) {
	block.Nonce = fields.VarUint4(n)
}
func (block *Block_v1) SetNonceByte(nonce []byte) {
	block.Nonce = fields.VarUint4(binary.BigEndian.Uint32(nonce))
}

func (block *Block_v1) SetTransactions(trslist []interfacev2.Transaction) {
	trsset := make([]interfaces.Transaction, len(block.Transactions))
	for i, v := range trslist {
		trsset[i] = v.(interfaces.Transaction)
	}
	block.Transactions = trsset
}

func (block *Block_v1) GetTransactions() []interfacev2.Transaction {
	trslist := make([]interfacev2.Transaction, len(block.Transactions))
	for i, v := range block.Transactions {
		trslist[i] = v.(interfacev2.Transaction)
	}
	return trslist
}

func (block *Block_v1) AddTransaction(trs interfacev2.Transaction) {
	block.Transactions = append(block.Transactions, trs.(interfaces.Transaction))
	block.TransactionCount += 1
}

func (block *Block_v1) SetTrsList(trslist []interfaces.Transaction) {
	block.Transactions = trslist
}

func (block *Block_v1) GetTrsList() []interfaces.Transaction {
	return block.Transactions
}

func (block *Block_v1) AddTrs(trs interfaces.Transaction) {
	block.Transactions = append(block.Transactions, trs)
	block.TransactionCount += 1
}

// 验证需要的签名
func (block *Block_v1) VerifyNeedSigns() (bool, error) {
	for _, tx := range block.Transactions {
		ok, e := tx.VerifyAllNeedSigns()
		if !ok || e != nil {
			return ok, e // 验证失败
		}
	}
	return true, nil
}

func (block *Block_v1) WriteInChainState(blockstate interfaces.ChainStateOperation) error {
	blkhei := block.GetHeight()
	txlen := len(block.Transactions)
	totalfeeuserpay := fields.NewEmptyAmount()
	totalfeeminergot := fields.NewEmptyAmount()
	// 第一条交易为coinbase交易，客户交易从第二条开始
	for i := 1; i < txlen; i++ {
		tx := block.Transactions[i]
		txhx := tx.Hash()
		// 检查交易是否已经上链
		ishav, e := blockstate.CheckTxHash(txhx)
		if e != nil {
			return e // 验证失败
		}
		// 问题修复： 63448 区块将同一笔交易包含了两次
		if ishav && blkhei != 63448 {
			// 交易已经上链
			return fmt.Errorf("Tx <%s> is exist, block %d.", txhx.ToHex())
		}
		// 执行上链
		e = blockstate.ContainTxHash(txhx, fields.BlockHeight(blkhei))
		if e != nil {
			return e
		}
		// 执行交易
		e = tx.(interfaces.Transaction).WriteInChainState(blockstate)
		if e != nil {
			return e // 验证失败
		}
		var feepay = tx.GetFee()
		var feegot = tx.GetFeeOfMinerRealReceived()
		totalfeeuserpay, e = totalfeeuserpay.Add(feepay)
		if e != nil {
			return e // 验证失败
		}
		totalfeeminergot, e = totalfeeminergot.Add(feegot)
		if e != nil {
			return e // 验证失败
		}
	}
	// coinbase
	if txlen < 1 {
		return fmt.Errorf("not find coinbase tx")
	}
	tx0 := block.Transactions[0]
	if tx0.Type() != 0 {
		return fmt.Errorf("transaction[0] not coinbase tx")
	}
	coinbase, ok := tx0.(*transactions.Transaction_0_Coinbase)
	if !ok {
		return fmt.Errorf("transaction[0] not coinbase tx")
	}
	coinbase.TotalFeeUserPayed = *totalfeeuserpay      // 支付总手续费
	coinbase.TotalFeeMinerReceived = *totalfeeminergot // 收到总手续费
	// coinbase change state
	e3 := coinbase.WriteInChainState(blockstate)
	if e3 != nil {
		return e3
	}

	// ok
	return nil
}

// 修改 / 恢复 状态数据库
func (block *Block_v1) WriteinChainState(blockstate interfacev2.ChainStateOperation) error {
	blkhei := block.GetHeight()
	txlen := len(block.Transactions)
	totalfeeuserpay := fields.NewEmptyAmount()
	totalfeeminergot := fields.NewEmptyAmount()
	// 第一条交易为coinbase交易，客户交易从第二条开始
	for i := 1; i < txlen; i++ {
		tx := block.Transactions[i]
		txhx := tx.Hash()
		// 检查交易是否已经上链
		ishav, e := blockstate.CheckTxHash(txhx)
		if e != nil {
			return e // 验证失败
		}
		// 问题修复： 63448 区块将同一笔交易包含了两次
		if ishav && blkhei != 63448 {
			// 交易已经上链
			return fmt.Errorf("Tx <%s> is exist, block %d.", txhx.ToHex())
		}
		// 执行上链
		e = blockstate.ContainTxHash(txhx, fields.BlockHeight(blkhei))
		if e != nil {
			return e
		}
		// 执行交易
		e = tx.(interfacev2.Transaction).WriteinChainState(blockstate)
		if e != nil {
			return e // 验证失败
		}
		var feepay = tx.GetFee()
		var feegot = tx.GetFeeOfMinerRealReceived()
		totalfeeuserpay, e = totalfeeuserpay.Add(feepay)
		if e != nil {
			return e // 验证失败
		}
		totalfeeminergot, e = totalfeeminergot.Add(feegot)
		if e != nil {
			return e // 验证失败
		}
	}
	// coinbase
	if txlen < 1 {
		return fmt.Errorf("not find coinbase tx")
	}
	tx0 := block.Transactions[0]
	if tx0.Type() != 0 {
		return fmt.Errorf("transaction[0] not coinbase tx")
	}
	coinbase, ok := tx0.(*transactions.Transaction_0_Coinbase)
	if !ok {
		return fmt.Errorf("transaction[0] not coinbase tx")
	}
	coinbase.TotalFeeUserPayed = *totalfeeuserpay      // 支付总手续费
	coinbase.TotalFeeMinerReceived = *totalfeeminergot // 收到总手续费
	// coinbase change state
	e3 := coinbase.WriteinChainState(blockstate)
	if e3 != nil {
		return e3
	}

	// ok
	return nil

}

func (block *Block_v1) RecoverChainState(blockstate interfacev2.ChainStateOperation) error {

	panic("RecoverChainState be deprecated")

	txlen := len(block.Transactions)
	totalfeeuserpay := fields.NewEmptyAmount()
	totalfeeminergot := fields.NewEmptyAmount()
	store := blockstate.BlockStore()
	// 倒序从最后一笔交易开始 Recover
	for i := txlen - 1; i > 0; i-- {
		tx := block.Transactions[i]
		e := tx.(interfacev2.Transaction).RecoverChainState(blockstate)
		if e != nil {
			return e // 失败
		}
		var feepay = tx.GetFee()
		var feegot = tx.GetFeeOfMinerRealReceived()
		totalfeeuserpay, e = totalfeeuserpay.Add(feepay)
		if e != nil {
			return e // 验证失败
		}
		totalfeeminergot, e = totalfeeminergot.Add(feegot)
		if e != nil {
			return e // 验证失败
		}
		// delete tx from db
		delerr := store.DeleteTransactionByHash(tx.Hash())
		if delerr != nil {
			return delerr
		}
	}
	coinbase, _ := block.Transactions[0].(*transactions.Transaction_0_Coinbase)
	coinbase.TotalFeeUserPayed = *totalfeeuserpay      // 支付总手续费
	coinbase.TotalFeeMinerReceived = *totalfeeminergot // 收到总手续费
	// coinbase change state
	e3 := coinbase.RecoverChainState(blockstate)
	if e3 != nil {
		return e3
	}
	// ok
	return nil
}
