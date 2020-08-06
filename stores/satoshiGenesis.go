package stores

import (
	"github.com/hacash/core/fields"
)

type SatoshiGenesis struct {
	TransferNo               fields.VarInt4 // 转账流水编号
	BitcoinBlockHeight       fields.VarInt4 // 转账的比特币区块高度
	BitcoinBlockTimestamp    fields.VarInt4 // 转账的比特币区块时间戳
	BitcoinEffectiveGenesis  fields.VarInt4 // 在这笔之前已经成功转移的比特币数量
	BitcoinQuantity          fields.VarInt4 // 本笔转账的比特币数量（单位：枚）
	AdditionalTotalHacAmount fields.VarInt4 // 本次转账[总共]应该增发的 hac 数量 （单位：枚）
	OriginAddress            fields.Address // 转出的比特币来源地址
	BitcoinTransferHash      fields.Hash    // 比特币转账交易哈希
}
