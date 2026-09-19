package request

type ConvertWalletRequest struct {
	FromAsset string `json:"from_asset"`
	ToAsset   string `json:"to_asset"`
	Amount    int64  `json:"amount"`
}
