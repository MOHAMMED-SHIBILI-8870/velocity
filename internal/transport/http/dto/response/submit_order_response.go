package response

type SubmitOrderResponse struct {
	OrderID int64  `json:"order_id,string"`
	Status  string `json:"status"`
	Symbol  string `json:"symbol"`
}
