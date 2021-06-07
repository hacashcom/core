package actions

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/hacash/core/fields"
	"github.com/hacash/core/interfaces"
	"github.com/hacash/core/stores"
	"github.com/hacash/core/sys"
	"math/big"
)

const (
	// 借贷周期区块数量
	DiamondsSystemLendingBorrowPeriodBlockNumber uint64 = 10000
)

/*

创建钻石借贷流程：

. 检查合约ID格式
. 检查合约是否已经存在
. 检查钻石状态是否可以借贷，是否归属抵押者
. 检查借贷HAC额度是否匹配
. 检查借贷时间周期 1~20

. 修改钻石抵押状态
. 扣除用户钻石余额
. 增加用户HAC余额
. 保存借贷合约

. 统计实时借贷数量
. 累计借出额度

*/

// 钻石系统借贷
type Action_15_DiamondsSystemLendingCreate struct {
	//
	LendingID           fields.Bytes14              // 借贷合约ID
	MortgageDiamondList fields.DiamondListMaxLen200 // 抵押钻石列表
	LoanTotalAmount     fields.Amount               // 总共借出HAC额度，必须等于总可借额度，不能多也不能少
	BorrowPeriod        fields.VarUint1             // 借款周期，一个周期代表 0.5%利息和10000个区块约35天，最低1最高20

	// data ptr
	belong_trs interfaces.Transaction
}

func (elm *Action_15_DiamondsSystemLendingCreate) Kind() uint16 {
	return 15
}

// json api
func (elm *Action_15_DiamondsSystemLendingCreate) Describe() map[string]interface{} {
	var data = map[string]interface{}{}
	return data
}

func (elm *Action_15_DiamondsSystemLendingCreate) Serialize() ([]byte, error) {
	var kindByte = make([]byte, 2)
	binary.BigEndian.PutUint16(kindByte, elm.Kind())
	var buffer bytes.Buffer
	buffer.Write(kindByte)
	var b1, _ = elm.LendingID.Serialize()
	var b2, _ = elm.MortgageDiamondList.Serialize()
	var b3, _ = elm.LoanTotalAmount.Serialize()
	var b4, _ = elm.BorrowPeriod.Serialize()
	buffer.Write(b1)
	buffer.Write(b2)
	buffer.Write(b3)
	buffer.Write(b4)
	return buffer.Bytes(), nil
}

func (elm *Action_15_DiamondsSystemLendingCreate) Parse(buf []byte, seek uint32) (uint32, error) {
	var e error = nil
	seek, e = elm.LendingID.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = elm.MortgageDiamondList.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = elm.LoanTotalAmount.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = elm.BorrowPeriod.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	return seek, nil
}

func (elm *Action_15_DiamondsSystemLendingCreate) Size() uint32 {
	return 2 +
		elm.LendingID.Size() +
		elm.MortgageDiamondList.Size() +
		elm.LoanTotalAmount.Size() +
		elm.BorrowPeriod.Size()
}

func (*Action_15_DiamondsSystemLendingCreate) RequestSignAddresses() []fields.Address {
	return []fields.Address{} // not sign
}

func (act *Action_15_DiamondsSystemLendingCreate) WriteinChainState(state interfaces.ChainStateOperation) error {

	if !sys.TestDebugLocalDevelopmentMark {
		return fmt.Errorf("mainnet not yet") // 暂未启用等待review
	}

	if act.belong_trs == nil {
		panic("Action belong to transaction not be nil !")
	}

	feeAddr := act.belong_trs.GetAddress()

	// 检查id格式
	if len(act.LendingID) != stores.DiamondLendingIdLength ||
		act.LendingID[0] == 0 ||
		act.LendingID[stores.DiamondLendingIdLength-1] == 0 {
		return fmt.Errorf("Diamond Lending Id format error.")
	}

	// 查询id是否存在
	dmdlendObj := state.DiamondLending(act.LendingID)
	if dmdlendObj != nil {
		return fmt.Errorf("Diamond Lending <%d> already exist.", hex.EncodeToString(act.LendingID))
	}

	// 数量检查
	dianum := int(act.MortgageDiamondList.Count)
	if dianum == 0 || dianum != len(act.MortgageDiamondList.Diamonds) {
		return fmt.Errorf("Diamonds quantity error")
	}
	if dianum > 200 {
		return fmt.Errorf("Diamonds quantity cannot over 200")
	}

	// 检查周期数
	if act.BorrowPeriod < 1 || act.BorrowPeriod > 20 {
		return fmt.Errorf("BorrowPeriod must between 1 ~ 20")
	}

	// 可借出HAC
	totalLoanHAC := int64(0)

	// 批量抵押钻石
	for i := 0; i < len(act.MortgageDiamondList.Diamonds); i++ {
		diamond := act.MortgageDiamondList.Diamonds[i]

		// 查询钻石是否存在
		diaitem := state.Diamond(diamond)
		if diaitem == nil {
			return fmt.Errorf("Diamond <%s> not find.", string(diamond))
		}
		item := diaitem
		// 检查是否已经抵押，是否可以抵押
		if diaitem.Status != stores.DiamondStatusNormal {
			return fmt.Errorf("Diamond <%s> has been mortgaged and cannot be transferred.", string(diamond))
		}
		// 检查所属
		if bytes.Compare(item.Address, feeAddr) != 0 {
			return fmt.Errorf("Diamond <%s> not belong to address '%s'", string(diamond), feeAddr.ToReadable())
		}
		// 标记抵押钻石
		item.Status = stores.DiamondStatusLendingSystem // 抵押给系统
		e5 := state.DiamondSet(diamond, item)
		if e5 != nil {
			return e5
		}
		diasmelt, e6 := state.BlockStore().ReadDiamond(diamond)
		if e6 != nil {
			return e5
		}
		if diasmelt == nil {
			return fmt.Errorf("Diamond <%s> not exist.", string(diamond))
		}

		// 统计可借出HAC数量
		totalLoanHAC += int64(diasmelt.AverageBidBurnPrice)
	}

	// 共借出 HAC 枚
	totalAmt := fields.NewAmountByUnit248(totalLoanHAC)
	// 验证数量
	if totalAmt.Equal(&act.LoanTotalAmount) == false {
		return fmt.Errorf("LoanTotalAmountMei <%s> and <%s> not match.", totalAmt.ToFinString(), act.LoanTotalAmount.ToFinString())
	}

	// 减少钻石余额
	e9 := DoSubDiamondFromChainState(state, feeAddr, fields.VarUint3(dianum))
	if e9 != nil {
		return e9
	}

	// 抵押成功，发放余额
	e10 := DoAddBalanceFromChainState(state, feeAddr, act.LoanTotalAmount)
	if e10 != nil {
		return e10
	}

	// 保存钻石抵押
	paddingHei := state.GetPendingBlockHeight()
	dlsto := &stores.DiamondLending{
		IsRansomed:          fields.CreateBool(false), // 标记未赎回
		CreateBlockHeight:   fields.VarUint5(paddingHei),
		MainAddress:         feeAddr,
		MortgageDiamondList: act.MortgageDiamondList,
		LoanTotalAmountMei:  fields.VarUint4(totalLoanHAC),
		BorrowPeriod:        act.BorrowPeriod,
	}
	e11 := state.DiamondLendingCreate(act.LendingID, dlsto)
	if e11 != nil {
		return e11
	}

	// 系统统计
	totalsupply, e20 := state.ReadTotalSupply()
	if e20 != nil {
		return e20
	}
	// 增加实时钻石系统抵押数量统计
	totalsupply.DoAdd(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCurrentMortgageCount,
		float64(dianum),
	)
	// 钻石系统抵押数量统计 累计借出流水
	totalsupply.DoAdd(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCumulationLoanHacAmount,
		act.LoanTotalAmount.ToMei(),
	)
	// 更新统计
	e21 := state.UpdateSetTotalSupply(totalsupply)
	if e21 != nil {
		return e21
	}

	// 完毕
	return nil
}

func (act *Action_15_DiamondsSystemLendingCreate) RecoverChainState(state interfaces.ChainStateOperation) error {

	if !sys.TestDebugLocalDevelopmentMark {
		return fmt.Errorf("mainnet not yet") // 暂未启用等待review
	}

	if act.belong_trs == nil {
		panic("Action belong to transaction not be nil !")
	}

	// 回退所有抵押
	feeAddr := act.belong_trs.GetAddress()

	// 批量抵押钻石
	for i := 0; i < len(act.MortgageDiamondList.Diamonds); i++ {
		diamond := act.MortgageDiamondList.Diamonds[i]
		// 查询钻石是否存在
		diaitem := state.Diamond(diamond)
		// 标记抵押钻石
		diaitem.Status = stores.DiamondStatusNormal // 恢复
		e5 := state.DiamondSet(diamond, diaitem)
		if e5 != nil {
			return e5
		}
	}

	// 回退钻石余额
	dianum := act.MortgageDiamondList.Count
	e9 := DoAddDiamondFromChainState(state, feeAddr, fields.VarUint3(dianum))
	if e9 != nil {
		return e9
	}

	// 取回HAC余额
	e10 := DoSubBalanceFromChainState(state, feeAddr, act.LoanTotalAmount)
	if e10 != nil {
		return e10
	}

	// 系统统计
	totalsupply, e20 := state.ReadTotalSupply()
	if e20 != nil {
		return e20
	}
	// 减少实时钻石系统抵押数量统计 回退
	totalsupply.DoSub(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCurrentMortgageCount,
		float64(dianum),
	)
	// 钻石系统抵押数量统计 累计借出流水 回退
	totalsupply.DoSub(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCumulationLoanHacAmount,
		act.LoanTotalAmount.ToMei(),
	)
	// 更新统计
	e21 := state.UpdateSetTotalSupply(totalsupply)
	if e21 != nil {
		return e21
	}

	return nil
}

// 设置所属 belong_trs
func (act *Action_15_DiamondsSystemLendingCreate) SetBelongTransaction(trs interfaces.Transaction) {
	act.belong_trs = trs
}

// burning fees  // 是否销毁本笔交易的 90% 的交易费用
func (act *Action_15_DiamondsSystemLendingCreate) IsBurning90PersentTxFees() bool {
	return false
}

/////////////////////////////////////////////////

/*

赎回钻石流程

. 检查抵押ID格式
. 检查抵押合约存在和状态
. 检查每个钻石状态
. 检查是否在私有赎回期
. 检查公共赎回期，并计算利息拍卖扣除
. 计算真实所需的赎回金额

. 检查赎回者HAC余额，并扣除赎回金额
. 增加用户钻石统计
. 修改每枚钻石状态
. 修改借贷合约状态

. 修改钻石抵押实时统计
. 累计赎回销毁HAC数量

*/

// 钻石系统借贷，赎回
type Action_16_DiamondsSystemLendingRansom struct {
	//
	LendingID    fields.Bytes14 // 借贷合约ID
	RansomAmount fields.Amount  // 赎回金额
	// data ptr
	belong_trs interfaces.Transaction
}

func (elm *Action_16_DiamondsSystemLendingRansom) Kind() uint16 {
	return 16
}

// json api
func (elm *Action_16_DiamondsSystemLendingRansom) Describe() map[string]interface{} {
	var data = map[string]interface{}{}
	return data
}

func (elm *Action_16_DiamondsSystemLendingRansom) Serialize() ([]byte, error) {
	var kindByte = make([]byte, 2)
	binary.BigEndian.PutUint16(kindByte, elm.Kind())
	var buffer bytes.Buffer
	buffer.Write(kindByte)
	var b1, _ = elm.LendingID.Serialize()
	var b2, _ = elm.RansomAmount.Serialize()
	buffer.Write(b1)
	buffer.Write(b2)
	return buffer.Bytes(), nil
}

func (elm *Action_16_DiamondsSystemLendingRansom) Parse(buf []byte, seek uint32) (uint32, error) {
	var e error = nil
	seek, e = elm.LendingID.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	seek, e = elm.RansomAmount.Parse(buf, seek)
	if e != nil {
		return 0, e
	}
	return seek, nil
}

func (elm *Action_16_DiamondsSystemLendingRansom) Size() uint32 {
	return 2 +
		elm.LendingID.Size() +
		elm.RansomAmount.Size()
}

func (*Action_16_DiamondsSystemLendingRansom) RequestSignAddresses() []fields.Address {
	return []fields.Address{} // not sign
}

func (act *Action_16_DiamondsSystemLendingRansom) WriteinChainState(state interfaces.ChainStateOperation) error {

	if !sys.TestDebugLocalDevelopmentMark {
		return fmt.Errorf("mainnet not yet") // 暂未启用等待review
	}

	// 借贷周期
	dslbpbn := DiamondsSystemLendingBorrowPeriodBlockNumber

	// 测试使用
	if sys.TestDebugLocalDevelopmentMark {
		dslbpbn = 50 // 测试时使用 50 个区块为一个周期
	}

	if act.belong_trs == nil {
		panic("Action belong to transaction not be nil !")
	}

	paddingHeight := state.GetPendingBlockHeight()
	feeAddr := act.belong_trs.GetAddress()

	// 检查id格式
	if len(act.LendingID) != stores.DiamondLendingIdLength ||
		act.LendingID[0] == 0 ||
		act.LendingID[stores.DiamondLendingIdLength-1] == 0 {
		return fmt.Errorf("Diamond Lending Id format error.")
	}

	// 查询id是否存在
	dmdlendObj := state.DiamondLending(act.LendingID)
	if dmdlendObj == nil {
		return fmt.Errorf("Diamond Lending <%d> not exist.", hex.EncodeToString(act.LendingID))
	}

	// 检查是否赎回状态
	if dmdlendObj.IsRansomed.Check() {
		// 已经赎回。不可再次赎回
		return fmt.Errorf("Diamond Lending <%d> has been redeemed.", hex.EncodeToString(act.LendingID))

	}

	// 赎回期基础
	ransomBlockNumberBase := uint64(dmdlendObj.BorrowPeriod) * dslbpbn

	// 检查私有赎回期
	privateHeight := uint64(dmdlendObj.CreateBlockHeight) + ransomBlockNumberBase
	if paddingHeight <= privateHeight && feeAddr.Equal(dmdlendObj.MainAddress) == false {
		// 未到期之前只能被抵押者私下赎回
		return fmt.Errorf("It can only be redeemed privately by the mortgagor %s before the blockheight %d", dmdlendObj.MainAddress.ToReadable(), privateHeight)
	}

	// 任何人可以公开赎回

	// 计算赎回金额（每一个周期表示 0.005 即 0.5% 的利息）
	realRansom1qian := (1000 + (5 * int64(dmdlendObj.BorrowPeriod))) * int64(dmdlendObj.LoanTotalAmountMei)

	// 检查公共赎回期
	publicHeight := privateHeight + ransomBlockNumberBase
	if paddingHeight > publicHeight {
		// 大于公共赎回期，开始利息荷兰拍卖模式
		subcount := int64((paddingHeight - publicHeight) / dslbpbn)
		maxsub := int64(dmdlendObj.BorrowPeriod) * 2
		if subcount > maxsub {
			subcount = maxsub // 减扣利息最大只能等于原始支付利息
		}
		// 减去利息拍卖
		realRansom1qian -= 5 * subcount * int64(dmdlendObj.LoanTotalAmountMei)
	}

	// 真实有效赎回金额
	validRansomAmt, e1 := fields.NewAmountByBigIntWithUnit(big.NewInt(realRansom1qian), 245)
	if e1 != nil {
		return e1
	}

	// 检查赎回金额是否有效（赎回金额真的大于实时计算的可赎回金额）
	if act.RansomAmount.LessThan(validRansomAmt) {
		return fmt.Errorf("Valid ransom amount must not less than %s but got %s", validRansomAmt.ToFinString(), act.RansomAmount.ToFinString())
	}

	// 赎回操作，扣除HAC余额（以便首先检查余额是否充足）
	e2 := DoAddBalanceFromChainState(state, feeAddr, act.RansomAmount)
	if e2 != nil {
		return e2
	}

	// 操作赎回
	dianum := dmdlendObj.MortgageDiamondList.Count

	// 批量赎回钻石
	for i := 0; i < len(dmdlendObj.MortgageDiamondList.Diamonds); i++ {
		diamond := dmdlendObj.MortgageDiamondList.Diamonds[i]
		// 查询钻石是否存在
		diaitem := state.Diamond(diamond)
		if diaitem == nil {
			return fmt.Errorf("diamond <%s> not find.", string(diamond))
		}
		if diaitem.Status != stores.DiamondStatusLendingSystem {
			return fmt.Errorf("diamond <%s> status is not [stores.DiamondStatusLendingSystem].", string(diamond))
		}
		// 标记赎回钻石
		diaitem.Status = stores.DiamondStatusNormal // 赎回钻石状态
		diaitem.Address = feeAddr                   // 钻石归属修改
		e5 := state.DiamondSet(diamond, diaitem)    // 更新钻石
		if e5 != nil {
			return e5
		}
	}

	// 增加钻石余额
	e9 := DoSubDiamondFromChainState(state, feeAddr, fields.VarUint3(dianum))
	if e9 != nil {
		return e9
	}

	// 修改抵押合约状态
	dmdlendObj.IsRansomed.Set(true) // 标记已经赎回，避免重复赎回
	e10 := state.DiamondLendingUpdate(act.LendingID, dmdlendObj)
	if e10 != nil {
		return e10
	}

	// 系统统计
	totalsupply, e20 := state.ReadTotalSupply()
	if e20 != nil {
		return e20
	}
	// 减少实时钻石系统抵押数量统计，实时减扣
	totalsupply.DoSub(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCurrentMortgageCount,
		float64(dianum),
	)
	// 钻石系统抵押数量统计 累计赎回流水
	totalsupply.DoAdd(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCumulationRansomHacAmount,
		act.RansomAmount.ToMei(),
	)
	// 更新统计
	e21 := state.UpdateSetTotalSupply(totalsupply)
	if e21 != nil {
		return e21
	}

	// 完毕
	return nil
}

func (act *Action_16_DiamondsSystemLendingRansom) RecoverChainState(state interfaces.ChainStateOperation) error {

	if !sys.TestDebugLocalDevelopmentMark {
		return fmt.Errorf("mainnet not yet") // 暂未启用等待review
	}

	if act.belong_trs == nil {
		panic("Action belong to transaction not be nil !")
	}

	feeAddr := act.belong_trs.GetAddress()

	// 回退所有赎回
	dmdlendObj := state.DiamondLending(act.LendingID)
	if dmdlendObj == nil {
		return fmt.Errorf("Diamond Lending <%d> not exist.", hex.EncodeToString(act.LendingID))
	}

	// 删除抵押合约
	state.DiamondLendingDelete(act.LendingID)

	// 钻石批量恢复抵押
	for i := 0; i < len(dmdlendObj.MortgageDiamondList.Diamonds); i++ {
		diamond := dmdlendObj.MortgageDiamondList.Diamonds[i]
		// 查询钻石是否存在
		diaitem := state.Diamond(diamond)
		// 标记抵押钻石
		diaitem.Status = stores.DiamondStatusLendingSystem // 状态依旧抵押
		diaitem.Address = dmdlendObj.MainAddress           // 钻石仍然归属抵押者
		e5 := state.DiamondSet(diamond, diaitem)
		if e5 != nil {
			return e5
		}
	}

	// 回退钻石余额
	dianum := dmdlendObj.MortgageDiamondList.Count
	e9 := DoSubDiamondFromChainState(state, feeAddr, fields.VarUint3(dianum))
	if e9 != nil {
		return e9
	}

	// 取回用于赎回的HAC余额
	e10 := DoAddBalanceFromChainState(state, feeAddr, act.RansomAmount)
	if e10 != nil {
		return e10
	}

	// 系统统计
	totalsupply, e20 := state.ReadTotalSupply()
	if e20 != nil {
		return e20
	}
	// 增加实时钻石系统抵押数量统计，增加，恢复
	totalsupply.DoAdd(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCurrentMortgageCount,
		float64(dianum),
	)
	// 钻石系统抵押数量统计 累计赎回流水， 减少， 回退
	totalsupply.DoSub(
		stores.TotalSupplyStoreTypeOfSystemLendingDiamondCumulationRansomHacAmount,
		act.RansomAmount.ToMei(),
	)
	// 更新统计
	e21 := state.UpdateSetTotalSupply(totalsupply)
	if e21 != nil {
		return e21
	}

	return nil
}

// 设置所属 belong_trs
func (act *Action_16_DiamondsSystemLendingRansom) SetBelongTransaction(trs interfaces.Transaction) {
	act.belong_trs = trs
}

// burning fees  // 是否销毁本笔交易的 90% 的交易费用
func (act *Action_16_DiamondsSystemLendingRansom) IsBurning90PersentTxFees() bool {
	return false
}
