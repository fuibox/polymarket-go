package data

// Position 表示用户当前持有的一个市场头寸（未平仓）。
// 对应 GET /positions 接口的单条响应。
type Position struct {
	// ProxyWallet 是用户在 Polymarket 的代理钱包地址（非用户主地址）。
	ProxyWallet string `json:"proxyWallet"`

	// Asset 是头寸对应的 ERC-1155 outcome token 地址（即 tokenId）。
	// 每个市场的每个 outcome（YES/NO）有独立的 Asset。
	Asset string `json:"asset"`

	// ConditionID 是该市场的条件 ID（condition ID），0x 开头的 hex 字符串。
	// 对应链上 CTF 合约的 conditionId，可用于赎回操作。
	ConditionID string `json:"conditionId"`

	// Size 是当前持仓数量（outcome token 数量，6 位精度）。
	Size float64 `json:"size"`

	// AvgPrice 是建仓均价（0~1 之间，代表概率）。
	AvgPrice float64 `json:"avgPrice"`

	// InitialValue 是建仓时的总成本（USDC，= Size × AvgPrice）。
	InitialValue float64 `json:"initialValue"`

	// CurrentValue 是当前持仓的市值（USDC，= Size × CurPrice）。
	CurrentValue float64 `json:"currentValue"`

	// CashPnl 是未实现盈亏（USDC，= CurrentValue - InitialValue）。
	CashPnl float64 `json:"cashPnl"`

	// PercentPnl 是未实现盈亏百分比（= CashPnl / InitialValue × 100）。
	PercentPnl float64 `json:"percentPnl"`

	// TotalBought 是历史累计买入的 token 数量（含已平仓部分）。
	TotalBought float64 `json:"totalBought"`

	// RealizedPnl 是已实现盈亏（USDC，来自已平仓或赎回的部分）。
	RealizedPnl float64 `json:"realizedPnl"`

	// PercentRealizedPnl 是已实现盈亏百分比。
	PercentRealizedPnl float64 `json:"percentRealizedPnl"`

	// CurPrice 是该 outcome token 的当前市场价格（0~1）。
	CurPrice float64 `json:"curPrice"`

	// Redeemable 表示该头寸是否可以赎回（市场已结算且该 outcome 获胜）。
	// true 时可调用 RelayClient.RedeemPosition() 进行链上赎回。
	Redeemable bool `json:"redeemable"`

	// Mergeable 表示该头寸是否可以合并（持有同一市场的互斥 outcome token）。
	// true 时可调用 RelayClient.MergePosition() 将 YES+NO 合并回 USDC。
	Mergeable bool `json:"mergeable"`

	// Title 是市场标题（如 "Will X happen by date?"）。
	Title string `json:"title"`

	// Slug 是市场的 URL slug，用于拼接 Polymarket 前端链接。
	Slug string `json:"slug"`

	// Icon 是市场图标的 URL。
	Icon string `json:"icon"`

	// EventID 是该市场所属事件的 ID。
	EventID string `json:"eventId"`

	// EventSlug 是该市场所属事件的 slug。
	EventSlug string `json:"eventSlug"`

	// Outcome 是本头寸对应的结果描述（通常为 "Yes" 或 "No"）。
	Outcome string `json:"outcome"`

	// OutcomeIndex 是本头寸在该市场所有 outcome 中的索引（0-based）。
	OutcomeIndex int `json:"outcomeIndex"`

	// OppositeOutcome 是与本头寸相对的结果描述（如持有 Yes，则为 "No"）。
	OppositeOutcome string `json:"oppositeOutcome"`

	// OppositeAsset 是与本头寸相对的 outcome token 地址。
	// Mergeable 为 true 时，需要同时持有 Asset 和 OppositeAsset。
	OppositeAsset string `json:"oppositeAsset"`

	// EndDate 是市场预期结算日期（RFC3339 格式），可能为空。
	EndDate *string `json:"endDate,omitempty"`

	// NegativeRisk 表示该市场是否为 NegRisk 类型（多结果互斥市场）。
	// NegRisk 市场的赎回和合并需使用专用合约，见 RelayClient.RedeemNegRiskPosition()。
	NegativeRisk *bool `json:"negativeRisk,omitempty"`
}

type ClosedPosition struct {
	ProxyWallet     string  `json:"proxyWallet"`
	Asset           string  `json:"asset"`
	ConditionID     string  `json:"conditionId"`
	Size            float64 `json:"size"`
	AvgPrice        float64 `json:"avgPrice"`
	RealizedPnl     float64 `json:"realizedPnl"`
	ClosedPrice     float64 `json:"closedPrice"`
	ClosedAt        string  `json:"closedAt"`
	Title           string  `json:"title"`
	Slug            string  `json:"slug"`
	Icon            string  `json:"icon"`
	EventID         string  `json:"eventId"`
	EventSlug       string  `json:"eventSlug"`
	Outcome         string  `json:"outcome"`
	OutcomeIndex    int     `json:"outcomeIndex"`
	OppositeOutcome string  `json:"oppositeOutcome"`
	OppositeAsset   string  `json:"oppositeAsset"`
	NegativeRisk    *bool   `json:"negativeRisk,omitempty"`
}

type DataTrade struct {
	ProxyWallet           string   `json:"proxyWallet"`
	Side                  string   `json:"side"`
	ConditionID           string   `json:"conditionId"`
	Outcome               string   `json:"outcome"`
	Market                string   `json:"market"`
	Size                  float64  `json:"size"`
	Price                 float64  `json:"price"`
	Fee                   *float64 `json:"fee,omitempty"`
	Timestamp             int64    `json:"timestamp"`
	TransactionHash       string   `json:"transactionHash"`
	Maker                 string   `json:"maker"`
	Taker                 string   `json:"taker"`
	AssetID               string   `json:"assetId"`
	Title                 string   `json:"title"`
	Slug                  string   `json:"slug"`
	Icon                  string   `json:"icon"`
	EventSlug             string   `json:"eventSlug"`
	OutcomeIndex          int      `json:"outcomeIndex"`
	Name                  string   `json:"name"`
	Pseudonym             string   `json:"pseudonym"`
	Bio                   string   `json:"bio"`
	ProfileImage          string   `json:"profileImage"`
	ProfileImageOptimized string   `json:"profileImageOptimized"`
}

type Activity struct {
	ProxyWallet           string   `json:"proxyWallet"`
	Timestamp             int64    `json:"timestamp"`
	Type                  string   `json:"type"`
	Size                  float64  `json:"size"`
	UsdcSize              float64  `json:"usdcSize"`
	Price                 *float64 `json:"price,omitempty"`
	Fee                   *float64 `json:"fee,omitempty"`
	ConditionID           string   `json:"conditionId"`
	Outcome               string   `json:"outcome"`
	Market                string   `json:"market"`
	TransactionHash       string   `json:"transactionHash"`
	From                  string   `json:"from"`
	To                    string   `json:"to"`
	AssetID               string   `json:"assetId"`
	Value                 *float64 `json:"value,omitempty"`
	Title                 string   `json:"title"`
	Slug                  string   `json:"slug"`
	Icon                  string   `json:"icon"`
	EventSlug             string   `json:"eventSlug"`
	OutcomeIndex          int      `json:"outcomeIndex"`
	Name                  string   `json:"name"`
	Pseudonym             string   `json:"pseudonym"`
	Bio                   string   `json:"bio"`
	ProfileImage          string   `json:"profileImage"`
	ProfileImageOptimized string   `json:"profileImageOptimized"`
}

type Holder struct {
	Wallet  string `json:"wallet"`
	Balance string `json:"balance"`
	Value   string `json:"value"`
}

type MetaHolder struct {
	Token   string   `json:"token"`
	Holders []Holder `json:"holders"`
}

type TotalValue struct {
	User  string  `json:"user"`
	Value float64 `json:"value"`
}

type TotalMarketsTraded struct {
	User   string `json:"user"`
	Traded int    `json:"traded"`
}

type OpenInterest struct {
	Market string  `json:"market"`
	Value  float64 `json:"value"`
}

type LiveVolumeMarket struct {
	Market string  `json:"market"`
	Value  float64 `json:"value"`
}

type LiveVolumeResponse struct {
	Total   int                `json:"total"`
	Markets []LiveVolumeMarket `json:"markets"`
}

type DataHealthResponse struct {
	Data string `json:"data"`
}

// PositionsQuery 是 GET /positions 的查询参数。
// User 为必填，其余均为可选过滤/排序条件。
// 注意：Market 与 EventID 互斥，不得同时传入。
type PositionsQuery struct {
	// User 是要查询的用户钱包地址（0x 开头）。必填。
	User *string `json:"user,omitempty"`

	// Market 按条件 ID（conditionId）过滤，逗号分隔，与 EventID 互斥。
	Market *[]string `json:"market,omitempty"`

	// EventID 按事件 ID 过滤，逗号分隔，与 Market 互斥。
	EventID *[]string `json:"eventId,omitempty"`

	// SizeThreshold 过滤掉持仓数量低于该值的记录。默认 1，最小 0。
	SizeThreshold *float64 `json:"sizeThreshold,omitempty"`

	// Redeemable 若为 true，只返回可赎回的头寸。默认 false。
	Redeemable *bool `json:"redeemable,omitempty"`

	// Mergeable 若为 true，只返回可合并的头寸。默认 false。
	Mergeable *bool `json:"mergeable,omitempty"`

	// Limit 每页返回条数，范围 0~500，默认 100。
	Limit *int `json:"limit,omitempty"`

	// Offset 分页偏移量，范围 0~10000，默认 0。
	Offset *int `json:"offset,omitempty"`

	// SortBy 排序字段。可选值：
	// CURRENT（当前市值）、INITIAL（建仓成本）、TOKENS（持仓数量）、
	// CASHPNL（绝对盈亏）、PERCENTPNL（百分比盈亏）、TITLE（标题）、
	// RESOLVING（即将结算）、PRICE（当前价格）、AVGPRICE（均价）。
	// 默认 TOKENS。
	SortBy *string `json:"sortBy,omitempty"`

	// SortDirection 排序方向：ASC 或 DESC。默认 DESC。
	SortDirection *string `json:"sortDirection,omitempty"`

	// Title 按市场标题模糊搜索，最长 100 个字符。
	Title *string `json:"title,omitempty"`
}

type ClosedPositionsQuery struct {
	User          *string   `json:"user,omitempty"`
	Market        *[]string `json:"market,omitempty"`
	EventID       *[]string `json:"eventId,omitempty"`
	Title         *string   `json:"title,omitempty"`
	Limit         *int      `json:"limit,omitempty"`
	Offset        *int      `json:"offset,omitempty"`
	SortBy        *string   `json:"sortBy,omitempty"`
	SortDirection *string   `json:"sortDirection,omitempty"`
}

type TradesQuery struct {
	Limit        *int      `json:"limit,omitempty"`
	Offset       *int      `json:"offset,omitempty"`
	TakerOnly    *bool     `json:"takerOnly,omitempty"`
	FilterType   *string   `json:"filterType,omitempty"`
	FilterAmount *float64  `json:"filterAmount,omitempty"`
	Market       *[]string `json:"market,omitempty"`
	EventID      *[]string `json:"eventId,omitempty"`
	User         *string   `json:"user,omitempty"`
	Side         *string   `json:"side,omitempty"`
}

type UserActivityQuery struct {
	User          *string   `json:"user,omitempty"`
	Limit         *int      `json:"limit,omitempty"`
	Offset        *int      `json:"offset,omitempty"`
	Market        *[]string `json:"market,omitempty"`
	EventID       *[]string `json:"eventId,omitempty"`
	Type          *string   `json:"type,omitempty"`
	Start         *string   `json:"start,omitempty"`
	End           *string   `json:"end,omitempty"`
	SortBy        *string   `json:"sortBy,omitempty"`
	SortDirection *string   `json:"sortDirection,omitempty"`
	Side          *string   `json:"side,omitempty"`
}

type TopHoldersQuery struct {
	Limit      *int     `json:"limit,omitempty"`      // 0-500, default 100
	Market     []string `json:"market"`               // Required, comma-separated condition IDs
	MinBalance *int     `json:"minBalance,omitempty"` // 0-999999, default 1
}

type TotalValueQuery struct {
	User   *string   `json:"user,omitempty"`
	Market *[]string `json:"market,omitempty"`
}

type TotalMarketsTradedQuery struct {
	User *string `json:"user,omitempty"`
}

type OpenInterestQuery struct {
	Market []string `json:"market"`
}

type LiveVolumeQuery struct {
	ID int `json:"id"`
}
