package actions

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/hacash/core/fields"
	"github.com/hacash/core/interfaces"
	"github.com/hacash/core/stores"
	"github.com/hacash/x16rs"
	"strings"
)

/**
 * 钻石交易类型
 */

// 挖出钻石
type Action_4_DiamondCreate struct {
	Diamond  fields.Bytes6  // 钻石字面量 WTYUIAHXVMEKBSZN
	Number   fields.VarInt3 // 钻石序号，用于难度检查
	PrevHash fields.Hash    // 上一个包含钻石的区块hash
	Nonce    fields.Bytes8  // 随机数
	Address  fields.Address // 所属账户

	// 数据指针
	// 所属交易
	belone_trs interfaces.Transaction
}

func (elm *Action_4_DiamondCreate) Kind() uint16 {
	return 4
}

func (elm *Action_4_DiamondCreate) Size() uint32 {
	return 2 +
		elm.Diamond.Size() +
		elm.Number.Size() +
		elm.PrevHash.Size() +
		elm.Nonce.Size() +
		elm.Address.Size()
}

func (elm *Action_4_DiamondCreate) Serialize() ([]byte, error) {
	var kindByte = make([]byte, 2)
	binary.BigEndian.PutUint16(kindByte, elm.Kind())
	var diamondBytes, _ = elm.Diamond.Serialize()
	var numberBytes, _ = elm.Number.Serialize()
	var prevBytes, _ = elm.PrevHash.Serialize()
	var nonceBytes, _ = elm.Nonce.Serialize()
	var addrBytes, _ = elm.Address.Serialize()
	var buffer bytes.Buffer
	buffer.Write(kindByte)
	buffer.Write(diamondBytes)
	buffer.Write(numberBytes)
	buffer.Write(prevBytes)
	buffer.Write(nonceBytes)
	buffer.Write(addrBytes)
	return buffer.Bytes(), nil
}

func (elm *Action_4_DiamondCreate) Parse(buf []byte, seek uint32) (uint32, error) {
	var moveseek1, _ = elm.Diamond.Parse(buf, seek)
	var moveseek2, _ = elm.Number.Parse(buf, moveseek1)
	var moveseek3, _ = elm.PrevHash.Parse(buf, moveseek2)
	var moveseek4, _ = elm.Nonce.Parse(buf, moveseek3)
	var moveseek5, _ = elm.Address.Parse(buf, moveseek4)
	return moveseek5, nil
}

func (elm *Action_4_DiamondCreate) RequestSignAddresses() []fields.Address {
	return []fields.Address{} // no sign
}

func (act *Action_4_DiamondCreate) WriteinChainState(state interfaces.ChainStateOperation) error {
	// 检查区块高度
	blkhei := state.GetPendingBlockHeight()
	// 检查区块高度值是否为5的倍数
	// {BACKTOPOOL} 表示扔回交易池等待下个区块再次处理
	if blkhei%5 != 0 {
		return fmt.Errorf("{BACKTOPOOL} Diamond must be in block height multiple of 5.")
	}
	// 矿工状态检查
	lastdiamond, err := state.ReadLastestDiamond()
	if err != nil {
		return err
	}
	if lastdiamond != nil {
		prevdiamondnum, prevdiamondhash := uint32(lastdiamond.Number), lastdiamond.ContainBlockHash
		// 检查钻石是否是从上一个区块得来
		if act.PrevHash.Equal(prevdiamondhash) != true {
			return fmt.Errorf("Diamond prev hash must be <%s> but got <%s>.", hex.EncodeToString(prevdiamondhash), hex.EncodeToString(act.PrevHash))
		}
		if prevdiamondnum+1 != uint32(act.Number) {
			return fmt.Errorf("Diamond number must be <%d> but got <%d>.", prevdiamondnum+1, act.Number)
		}
	}
	// 检查钻石挖矿计算
	diamond_resbytes, diamond_str := x16rs.Diamond(uint32(act.Number), act.PrevHash, act.Nonce, act.Address)
	diamondstrval, isdia := x16rs.IsDiamondHashResultString(diamond_str)
	if !isdia {
		return fmt.Errorf("String <%s> is not diamond.", diamond_str)
	}
	if strings.Compare(diamondstrval, string(act.Diamond)) != 0 {
		return fmt.Errorf("Diamond need <%s> but got <%s>", act.Diamond, diamondstrval)
	}
	// 检查钻石难度值
	difok := x16rs.CheckDiamondDifficulty(uint32(act.Number), diamond_resbytes)
	if !difok {
		return fmt.Errorf("Diamond difficulty not meet the requirements.")
	}
	// 查询钻石是否已经存在
	hasaddr := state.Diamond(act.Diamond)
	if hasaddr != nil {
		return fmt.Errorf("Diamond <%s> already exist.", string(act.Diamond))
	}
	// 检查一个区块只能包含一枚钻石
	pendingdiamond, e2 := state.GetPendingSubmitStoreDiamond()
	if e2 != nil {
		return e2
	}
	if pendingdiamond != nil {
		return fmt.Errorf("This block height:%d has already exist diamond:<%s> .", blkhei, pendingdiamond.Diamond)
	}
	// 存入钻石
	//fmt.Println(act.Address.ToReadable())
	var diastore stores.Diamond
	diastore.Address = act.Address
	e3 := state.DiamondSet(act.Diamond, &diastore) // 保存
	if e3 != nil {
		return e3
	}
	// 设置矿工状态
	//state.SetPrevDiamondHash(act.Number, )
	//标记本区块已经包含钻石
	var diamondstore = &stores.DiamondSmelt{
		Diamond:              act.Diamond,
		Number:               act.Number,
		ContainBlockHeight:   fields.VarInt5(blkhei),
		ContainBlockHash:     nil, // current block not exist !!!
		PrevContainBlockHash: act.PrevHash,
		MinerAddress:         act.Address,
		Nonce:                act.Nonce,
	}
	e4 := state.SetLastestDiamond(diamondstore)
	if e4 != nil {
		return e4
	}
	e5 := state.SetPendingSubmitStoreDiamond(diamondstore)
	if e5 != nil {
		return e5
	}

	//fmt.Println("Action_4_DiamondCreate:", diamondstore.Number, string(diamondstore.Diamond), diamondstore.MinerAddress.ToReadable())
	//fmt.Print(string(diamondstore.Diamond)+",")

	return nil
}

func (act *Action_4_DiamondCreate) RecoverChainState(state interfaces.ChainStateOperation) error {
	//chainstate := state.BlockStore()
	//if chainstate == nil {
	//	panic("Action get state.Miner() cannot be nil !")
	//}
	// 删除钻石
	e1 := state.DiamondDel(act.Diamond)
	if e1 != nil {
		return e1
	}
	// 回退矿工状态
	chainstore := state.BlockStore()
	if chainstore == nil {
		return fmt.Errorf("not find BlockStore object.")

	}
	prevDiamond, e2 := chainstore.ReadDiamondByNumber(uint32(act.Number) - 1)
	if e2 != nil {
		return e1
	}
	// setPrev
	e3 := state.SetLastestDiamond(prevDiamond)
	if e3 != nil {
		return e3
	}
	return nil
}

func (elm *Action_4_DiamondCreate) SetBelongTransaction(t interfaces.Transaction) {
	elm.belone_trs = t
}

///////////////////////////////////////////////////////////////

// 转移钻石
type Action_5_DiamondTransfer struct {
	Diamond fields.Bytes6  // 钻石字面量 WTYUIAHXVMEKBSZN
	Address fields.Address // 收钻方账户

	// 数据指针
	// 所属交易
	trs interfaces.Transaction
}

func (elm *Action_5_DiamondTransfer) Kind() uint16 {
	return 5
}

func (elm *Action_5_DiamondTransfer) Size() uint32 {
	return 2 + elm.Diamond.Size() + elm.Address.Size()
}

func (elm *Action_5_DiamondTransfer) Serialize() ([]byte, error) {
	var kindByte = make([]byte, 2)
	binary.BigEndian.PutUint16(kindByte, elm.Kind())
	var diamondBytes, _ = elm.Diamond.Serialize()
	var addrBytes, _ = elm.Address.Serialize()
	var buffer bytes.Buffer
	buffer.Write(kindByte)
	buffer.Write(diamondBytes)
	buffer.Write(addrBytes)
	return buffer.Bytes(), nil
}

func (elm *Action_5_DiamondTransfer) Parse(buf []byte, seek uint32) (uint32, error) {
	var moveseek1, _ = elm.Diamond.Parse(buf, seek)
	var moveseek2, _ = elm.Address.Parse(buf, moveseek1)
	return moveseek2, nil
}

func (elm *Action_5_DiamondTransfer) RequestSignAddresses() []fields.Address {
	return []fields.Address{} // not sign
}

func (act *Action_5_DiamondTransfer) WriteinChainState(state interfaces.ChainStateOperation) error {

	if act.trs == nil {
		panic("Action belong to transaction not be nil !")
	}

	trsMainAddress := act.trs.GetAddress()

	//fmt.Println("Action_5_DiamondTransfer:", trsMainAddress.ToReadable(), act.Address.ToReadable(), string(act.Diamond))

	// 自己不能转给自己
	if bytes.Compare(act.Address, trsMainAddress) == 0 {
		return fmt.Errorf("Cannot transfer to self.")
	}
	// 查询钻石是否已经存在
	diaitem := state.Diamond(act.Diamond)
	if diaitem == nil {
		return fmt.Errorf("Diamond <%s> not exist.", string(act.Diamond))
	}
	item := diaitem
	// 检查所属
	if bytes.Compare(item.Address, trsMainAddress) != 0 {
		return fmt.Errorf("Diamond <%s> not belong to belone_trs address.", string(act.Diamond))
	}
	// 转移钻石
	item.Address = act.Address
	err := state.DiamondSet(act.Diamond, item)
	if err != nil {
		return err
	}
	return nil
}

func (act *Action_5_DiamondTransfer) RecoverChainState(state interfaces.ChainStateOperation) error {
	if act.trs == nil {
		panic("Action belong to transaction not be nil !")
	}
	// get diamond
	diaitem := state.Diamond(act.Diamond)
	if diaitem == nil {
		return fmt.Errorf("Diamond <%s> not exist.", string(act.Diamond))
	}
	item := diaitem
	// 回退钻石
	item.Address = act.trs.GetAddress()
	err := state.DiamondSet(act.Diamond, item)
	if err != nil {
		return err
	}
	return nil
}

func (elm *Action_5_DiamondTransfer) SetBelongTransaction(t interfaces.Transaction) {
	elm.trs = t
}

///////////////////////////////////////////////////////////////

// 批量转移钻石
type Action_6_OutfeeQuantityDiamondTransfer struct {
	FromAddress  fields.Address  // 拥有钻石的账户
	ToAddress    fields.Address  // 收钻方账户
	DiamondCount fields.VarInt1  // 钻石数量
	Diamonds     []fields.Bytes6 // 钻石字面量数组

	// 数据指针
	// 所属交易
	trs interfaces.Transaction
}

func (elm *Action_6_OutfeeQuantityDiamondTransfer) Kind() uint16 {
	return 6
}

func (elm *Action_6_OutfeeQuantityDiamondTransfer) Size() uint32 {
	return 2 +
		elm.FromAddress.Size() +
		elm.ToAddress.Size() +
		elm.DiamondCount.Size() +
		uint32(len(elm.Diamonds))*6
}

func (elm *Action_6_OutfeeQuantityDiamondTransfer) Serialize() ([]byte, error) {
	if int(elm.DiamondCount) != len(elm.Diamonds) {
		return nil, fmt.Errorf("diamonds number quantity count error")
	}
	var kindByte = make([]byte, 2)
	binary.BigEndian.PutUint16(kindByte, elm.Kind())
	var addr1Bytes, _ = elm.FromAddress.Serialize()
	var addr2Bytes, _ = elm.ToAddress.Serialize()
	var countBytes, _ = elm.DiamondCount.Serialize()
	var buffer bytes.Buffer
	buffer.Write(kindByte)
	buffer.Write(addr1Bytes)
	buffer.Write(addr2Bytes)
	buffer.Write(countBytes)
	for _, v := range elm.Diamonds {
		diabts, _ := v.Serialize()
		buffer.Write(diabts)
	}
	return buffer.Bytes(), nil
}

func (elm *Action_6_OutfeeQuantityDiamondTransfer) Parse(buf []byte, seek uint32) (uint32, error) {
	seek, _ = elm.FromAddress.Parse(buf, seek)
	seek, _ = elm.ToAddress.Parse(buf, seek)
	seek, _ = elm.DiamondCount.Parse(buf, seek)
	elm.Diamonds = make([]fields.Bytes6, int(elm.DiamondCount))
	for i := 0; i < int(elm.DiamondCount); i++ {
		elm.Diamonds[i] = fields.Bytes6{}
		seek, _ = elm.Diamonds[i].Parse(buf, seek)
	}
	return seek, nil
}

func (elm *Action_6_OutfeeQuantityDiamondTransfer) RequestSignAddresses() []fields.Address {
	reqs := make([]fields.Address, 1) // 需from签名
	reqs[0] = elm.FromAddress
	return reqs
}

func (act *Action_6_OutfeeQuantityDiamondTransfer) WriteinChainState(state interfaces.ChainStateOperation) error {
	if act.trs == nil {
		panic("Action belong to transaction not be nil !")
	}
	// 数量检查
	if int(act.DiamondCount) != len(act.Diamonds) {
		return fmt.Errorf("Diamonds number quantity count error")
	}
	// 自己不能转给自己
	if bytes.Compare(act.FromAddress, act.ToAddress) == 0 {
		return fmt.Errorf("Cannot transfer to self.")
	}
	// 批量转移钻石
	for i := 0; i < len(act.Diamonds); i++ {
		diamond := act.Diamonds[i]

		//fmt.Println("Action_6_OutfeeQuantityDiamondTransfer:", act.FromAddress.ToReadable(), act.ToAddress.ToReadable(), string(diamond))

		// fmt.Println("--- " + string(diamond))
		// 查询钻石是否已经存在
		diaitem := state.Diamond(diamond)
		if diaitem == nil {
			//panic("Quantity Diamond <%s> not exist. " + string(diamond))
			return fmt.Errorf("Quantity Diamond <%s> not exist.", string(diamond))
		}
		item := diaitem
		// 检查所属
		if bytes.Compare(item.Address, act.FromAddress) != 0 {
			return fmt.Errorf("Diamond <%s> not belong to address '%s'", string(diamond), act.FromAddress.ToReadable())
		}
		// 转移钻石
		item.Address = act.ToAddress
		state.DiamondSet(diamond, item)
	}
	return nil
}

func (act *Action_6_OutfeeQuantityDiamondTransfer) RecoverChainState(state interfaces.ChainStateOperation) error {
	if act.trs == nil {
		panic("Action belong to transaction not be nil !")
	}
	// 批量回退钻石
	for i := 0; i < len(act.Diamonds); i++ {
		diamond := act.Diamonds[i]
		// get diamond
		diaitem := state.Diamond(diamond)
		if diaitem == nil {
			return fmt.Errorf("Diamond <%s> not exist.", string(diamond))
		}
		item := diaitem
		// 回退钻石
		item.Address = act.FromAddress
		err := state.DiamondSet(diamond, item)
		if err != nil {
			return err
		}
	}
	return nil
}

func (elm *Action_6_OutfeeQuantityDiamondTransfer) SetBelongTransaction(t interfaces.Transaction) {
	elm.trs = t
}
