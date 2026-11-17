package stockgo

func mapDragonDetail(r emRow) DragonTigerDetailItem {
	return DragonTigerDetailItem{
		Code:             rowText(r, "SECURITY_CODE"),
		Name:             rowText(r, "SECURITY_NAME_ABBR"),
		Date:             dcDate(rowText(r, "TRADE_DATE")),
		Close:            rawNumber(r["CLOSE_PRICE"]),
		ChangePercent:    rawNumber(r["CHANGE_RATE"]),
		NetBuyAmount:     rawNumber(r["BILLBOARD_NET_AMT"]),
		BuyAmount:        rawNumber(r["BILLBOARD_BUY_AMT"]),
		SellAmount:       rawNumber(r["BILLBOARD_SELL_AMT"]),
		DealAmount:       rawNumber(r["BILLBOARD_DEAL_AMT"]),
		TotalAmount:      rawNumber(r["ACCUM_AMOUNT"]),
		NetBuyRatio:      rawNumber(r["DEAL_NET_RATIO"]),
		DealAmountRatio:  rawNumber(r["DEAL_AMOUNT_RATIO"]),
		TurnoverRate:     rawNumber(r["TURNOVERRATE"]),
		FloatMarketValue: rawNumber(r["FREE_MARKET_CAP"]),
		Reason:           rowTextDefault(r, "EXPLANATION", "EXPLAIN"),
		AfterChange1d:    rawNumber(r["D1_CLOSE_ADJCHRATE"]),
		AfterChange2d:    rawNumber(r["D2_CLOSE_ADJCHRATE"]),
		AfterChange5d:    rawNumber(r["D5_CLOSE_ADJCHRATE"]),
		AfterChange10d:   rawNumber(r["D10_CLOSE_ADJCHRATE"]),
	}
}

func mapDragonStockStat(r emRow) DragonTigerStockStatItem {
	return DragonTigerStockStatItem{
		Code:            rowText(r, "SECURITY_CODE"),
		Name:            rowText(r, "SECURITY_NAME_ABBR"),
		LatestDate:      dcDate(rowText(r, "LATEST_TDATE")),
		Close:           rawNumber(r["CLOSE_PRICE"]),
		ChangePercent:   rawNumber(r["CHANGE_RATE"]),
		Count:           rawNumber(r["BILLBOARD_TIMES"]),
		TotalBuyAmount:  rawNumber(r["BILLBOARD_BUY_AMT"]),
		TotalSellAmount: rawNumber(r["BILLBOARD_SELL_AMT"]),
		TotalNetAmount:  rawNumber(r["BILLBOARD_NET_AMT"]),
		TotalDealAmount: rawNumber(r["BILLBOARD_DEAL_AMT"]),
		BuyOrgCount:     rawNumber(r["ORG_BUY_TIMES"]),
		SellOrgCount:    rawNumber(r["ORG_SELL_TIMES"]),
	}
}

func mapDragonInstitution(r emRow) DragonTigerInstitutionItem {
	return DragonTigerInstitutionItem{
		Code:          rowText(r, "SECURITY_CODE"),
		Name:          rowText(r, "SECURITY_NAME_ABBR"),
		Date:          dcDate(rowText(r, "TRADE_DATE")),
		Close:         rawNumber(r["CLOSE_PRICE"]),
		ChangePercent: rawNumber(r["CHANGE_RATE"]),
		BuyOrgCount:   rawNumber(r["BUY_TIMES"]),
		SellOrgCount:  rawNumber(r["SELL_TIMES"]),
		OrgBuyAmount:  rawNumber(r["BUY_AMT"]),
		OrgSellAmount: rawNumber(r["SELL_AMT"]),
		OrgNetAmount:  rawNumber(r["NET_AMT"]),
	}
}

func mapDragonBranch(r emRow) DragonTigerBranchItem {
	return DragonTigerBranchItem{
		Code:            rowText(r, "OPERATEDEPT_CODE"),
		Name:            rowText(r, "OPERATEDEPT_NAME"),
		TotalBuyAmount:  rawNumber(rawDefault(r, "TOTAL_BUYAMT", "BUY_AMT")),
		TotalSellAmount: rawNumber(rawDefault(r, "TOTAL_SELLAMT", "SELL_AMT")),
		BuyCount:        rawNumber(rawDefault(r, "TOTAL_BUYER_SALESTIMES", "BUY_TIMES")),
		SellCount:       rawNumber(rawDefault(r, "TOTAL_SELLER_SALESTIMES", "SELL_TIMES")),
		TotalCount:      rawNumber(r["TOTAL_TIMES"]),
	}
}

func mapBlockMarket(r emRow) BlockTradeMarketStatItem {
	return BlockTradeMarketStatItem{
		Date:            dcDate(rowText(r, "TRADE_DATE")),
		ShClose:         rawNumber(rawDefault(r, "CLOSE_PRICE", "SH_CLOSE_PRICE")),
		ShChangePercent: rawNumber(rawDefault(r, "CHANGE_RATE", "SH_CHANGE_RATE")),
		TotalAmount:     rawNumber(rawDefault(r, "TURNOVER", "TOTAL_AMOUNT")),
		PremiumAmount:   rawNumber(rawDefault(r, "PREMIUM_TURNOVER", "PREMIUM_AMOUNT")),
		PremiumRatio:    rawNumber(r["PREMIUM_RATIO"]),
		DiscountAmount:  rawNumber(rawDefault(r, "DISCOUNT_TURNOVER", "DISCOUNT_AMOUNT")),
		DiscountRatio:   rawNumber(r["DISCOUNT_RATIO"]),
	}
}

func mapBlockDetail(r emRow) BlockTradeDetailItem {
	return BlockTradeDetailItem{
		Code:          rowText(r, "SECURITY_CODE"),
		Name:          rowText(r, "SECURITY_NAME_ABBR"),
		Date:          dcDate(rowText(r, "TRADE_DATE")),
		Close:         rawNumber(r["CLOSE_PRICE"]),
		ChangePercent: rawNumber(r["CHANGE_RATE"]),
		DealPrice:     rawNumber(rawDefault(r, "DEAL_PRICE", "PRICE")),
		DealVolume:    rawNumber(rawDefault(r, "DEAL_VOLUME", "VOLUME")),
		DealAmount:    rawNumber(rawDefault(r, "DEAL_AMT", "TURNOVER")),
		PremiumRate:   rawNumber(rawDefault(r, "PREMIUM_RATIO", "PREMIUM_RATE")),
		BuyBranch:     rowTextDefault(r, "BUYER_DEPT", "BUYER_OPERATEDEPT_NAME"),
		SellBranch:    rowTextDefault(r, "SELLER_DEPT", "SELLER_OPERATEDEPT_NAME"),
	}
}

func mapBlockDaily(r emRow) BlockTradeDailyStatItem {
	return BlockTradeDailyStatItem{
		Code:            rowText(r, "SECURITY_CODE"),
		Name:            rowText(r, "SECURITY_NAME_ABBR"),
		Date:            dcDate(rowText(r, "TRADE_DATE")),
		ChangePercent:   rawNumber(r["CHANGE_RATE"]),
		Close:           rawNumber(r["CLOSE_PRICE"]),
		DealCount:       rawNumber(rawDefault(r, "DEAL_NUM", "DEAL_COUNT")),
		DealTotalAmount: rawNumber(rawDefault(r, "DEAL_AMT", "TOTAL_AMOUNT")),
		DealTotalVolume: rawNumber(rawDefault(r, "DEAL_VOLUME", "TOTAL_VOLUME")),
		PremiumAmount:   rawNumber(rawDefault(r, "PREMIUM_AMT", "PREMIUM_AMOUNT")),
		DiscountAmount:  rawNumber(rawDefault(r, "DISCOUNT_AMT", "DISCOUNT_AMOUNT")),
	}
}

func mapMarginAccount(r emRow) MarginAccountItem {
	return MarginAccountItem{
		Date:                   dcDate(rowTextDefault(r, "STATISTICS_DATE", "TRADE_DATE")),
		FinBalance:             rawNumber(r["FIN_BALANCE"]),
		LoanBalance:            rawNumber(r["LOAN_BALANCE"]),
		FinBuyAmount:           rawNumber(r["FIN_BUY_AMT"]),
		LoanSellAmount:         rawNumber(r["LOAN_SELL_AMT"]),
		InvestorCount:          rawNumber(rawDefault(r, "OPERATE_INVESTOR_NUM", "INVESTOR_NUM")),
		LiabilityInvestorCount: rawNumber(r["MARGIN_INVESTOR_NUM"]),
		TotalGuarantee:         rawNumber(r["TOTAL_GUARANTEE"]),
		AvgGuaranteeRatio:      rawNumber(r["AVG_GUARANTEE_RATIO"]),
	}
}

func mapMarginTarget(r emRow) MarginTargetItem {
	return MarginTargetItem{
		Code:            rowText(r, "SECURITY_CODE"),
		Name:            rowText(r, "SECURITY_NAME_ABBR"),
		Date:            dcDate(rowText(r, "TRADE_DATE")),
		FinBalance:      rawNumber(r["FIN_BALANCE"]),
		FinBuyAmount:    rawNumber(r["FIN_BUY_AMT"]),
		FinRepayAmount:  rawNumber(r["FIN_REPAY_AMT"]),
		LoanBalance:     rawNumber(r["LOAN_BALANCE"]),
		LoanSellVolume:  rawNumber(r["LOAN_SELL_VOLUME"]),
		LoanRepayVolume: rawNumber(r["LOAN_REPAY_VOLUME"]),
	}
}
