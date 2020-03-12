package client

import (
	"encoding/base64"
	"net/url"

	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/node/api"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TransactionPoolFeeGet uses the /tpool/fee endpoint to get a fee estimation.
func (c *Client) TransactionPoolFeeGet() (tfg api.TpoolFeeGET, err error) {
	err = c.get("/tpool/fee", &tfg)
	return
}

// TransactionPoolRawPost uses the /tpool/raw endpoint to send a raw
// transaction to the transaction pool.
func (c *Client) TransactionPoolRawPost(txn types.Transaction, parents []types.Transaction) (err error) {
	values := url.Values{}
	values.Set("transaction", base64.StdEncoding.EncodeToString(encoding.Marshal(txn)))
	values.Set("parents", base64.StdEncoding.EncodeToString(encoding.Marshal(parents)))
	err = c.post("/tpool/raw", values.Encode(), nil)
	return
}

// TransactionPoolTransactionsGet uses the /tpool/transactions endpoint to get the
// transactions of the tpool
func (c *Client) TransactionPoolTransactionsGet() (tptg api.TpoolTxnsGET, err error) {
	err = c.get("/tpool/transactions", &tptg)
	return
}
