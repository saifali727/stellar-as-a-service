package models

// WalletResponse represents the API response for wallet creation
type WalletResponse struct {
	PublicKey string `json:"public_key"`
	SecretKey string `json:"secret_key"`
	Message   string `json:"message"`
}

// WalletDetailsResponse represents the API response for wallet details
type WalletDetailsResponse struct {
	PublicKey string `json:"public_key"`
	Exists    bool   `json:"exists"`
	Balances  []struct {
		AssetType string `json:"asset_type"`
		AssetCode string `json:"asset_code,omitempty"`
		Issuer    string `json:"issuer,omitempty"`
		Balance   string `json:"balance"`
	} `json:"balances"`
	SequenceNumber int64 `json:"sequence_number"`
}

// TransferRequest represents the request body for the transfer endpoint
type TransferRequest struct {
	FromSecretKey string `json:"from_secret_key" binding:"required"`
	ToPublicKey   string `json:"to_public_key" binding:"required"`
	Amount        string `json:"amount" binding:"required"`
}

// TransferResponse represents the API response for the transfer endpoint
type TransferResponse struct {
	TransactionHash string `json:"transaction_hash"`
	Message         string `json:"message"`
}
