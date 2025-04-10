package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

// Config holds application configuration
type Config struct {
	Network       string
	MasterSecret  string
	HorizonClient *horizonclient.Client
	USDCAsset     txnbuild.CreditAsset // USDC asset
}

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

func main() {
	// Load configuration from environment variables
	config := Config{
		Network:      os.Getenv("STELLAR_NETWORK"),
		MasterSecret: os.Getenv("MASTER_SECRET_KEY"),
		USDCAsset: txnbuild.CreditAsset{
			Code:   "USDC",
			Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34KPPVPQS", // Testnet USDC issuer
		},
	}

	// Set Horizon client based on network
	if config.Network == "testnet" {
		config.HorizonClient = horizonclient.DefaultTestNetClient
	} else {
		config.HorizonClient = horizonclient.DefaultPublicNetClient
	}

	// Initialize Gin router
	router := gin.Default()

	// Wallet creation endpoint
	router.POST("/api/v1/wallets/create", func(c *gin.Context) {
		createWalletHandler(c, config)
	})

	// Wallet details endpoint
	router.GET("/api/v1/wallets/:public_key", func(c *gin.Context) {
		getWalletDetailsHandler(c, config)
	})

	// Wallet transfer endpoint
	router.POST("/api/v1/wallets/transfer", func(c *gin.Context) {
		transferFundsHandler(c, config)
	})

	// Run the server
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func createWalletHandler(c *gin.Context, config Config) {
	// Generate a new keypair for the wallet
	kp, err := keypair.Random()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate keypair"})
		return
	}

	publicKey := kp.Address()
	secretKey := kp.Seed()

	// Load the master account
	masterKP, err := keypair.Parse(config.MasterSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid master secret key"})
		return
	}

	// Operations: Create account + Trust USDC + Fund with USDC
	createAccountOp := txnbuild.CreateAccount{
		Destination: publicKey,
		Amount:      "0.5", // Minimum reserve only
	}

	// Convert USDCAsset to ChangeTrustAsset and handle error
	usdcChangeTrustAsset, err := config.USDCAsset.ToChangeTrustAsset()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create USDC trustline asset: " + err.Error()})
		return
	}
	trustOp := txnbuild.ChangeTrust{
		Line: usdcChangeTrustAsset,
	}

	paymentOp := txnbuild.Payment{
		Destination: publicKey,
		Amount:      "100", // Fund with 100 USDC
		Asset:       config.USDCAsset,
	}

	// Fetch master account details
	accountRequest := horizonclient.AccountRequest{AccountID: masterKP.Address()}
	sourceAccount, err := config.HorizonClient.AccountDetail(accountRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch master account details"})
		return
	}

	// Build transaction
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			Operations:           []txnbuild.Operation{&createAccountOp, &trustOp, &paymentOp},
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
			IncrementSequenceNum: true,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build transaction: " + err.Error()})
		return
	}

	// Sign transaction
	var networkPassphrase string
	if config.Network == "testnet" {
		networkPassphrase = network.TestNetworkPassphrase
	} else {
		networkPassphrase = network.PublicNetworkPassphrase
	}

	masterFullKP, ok := masterKP.(*keypair.Full)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Master key is not a full keypair"})
		return
	}
	tx, err = tx.Sign(networkPassphrase, masterFullKP, kp) // Sign with both master and new account
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign transaction"})
		return
	}

	// Submit transaction
	resp, err := config.HorizonClient.SubmitTransaction(tx)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed", "details": herr.Problem.Detail})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction: " + err.Error()})
		return
	}

	// Success response
	response := WalletResponse{
		PublicKey: publicKey,
		SecretKey: secretKey,
		Message:   "Wallet created, trusted USDC, and funded successfully. Hash: " + resp.Hash,
	}
	c.JSON(http.StatusOK, response)
}

func getWalletDetailsHandler(c *gin.Context, config Config) {
	publicKey := c.Param("public_key")

	if _, err := keypair.ParseAddress(publicKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid public key format"})
		return
	}

	accountRequest := horizonclient.AccountRequest{AccountID: publicKey}
	account, err := config.HorizonClient.AccountDetail(accountRequest)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok && herr.Response.StatusCode == http.StatusNotFound {
			response := WalletDetailsResponse{
				PublicKey: publicKey,
				Exists:    false,
				Balances: []struct {
					AssetType string `json:"asset_type"`
					AssetCode string `json:"asset_code,omitempty"`
					Issuer    string `json:"issuer,omitempty"`
					Balance   string `json:"balance"`
				}{},
				SequenceNumber: 0,
			}
			c.JSON(http.StatusOK, response)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch wallet details: " + err.Error()})
		return
	}

	var balances []struct {
		AssetType string `json:"asset_type"`
		AssetCode string `json:"asset_code,omitempty"`
		Issuer    string `json:"issuer,omitempty"`
		Balance   string `json:"balance"`
	}
	for _, balance := range account.Balances {
		balances = append(balances, struct {
			AssetType string `json:"asset_type"`
			AssetCode string `json:"asset_code,omitempty"`
			Issuer    string `json:"issuer,omitempty"`
			Balance   string `json:"balance"`
		}{
			AssetType: balance.Type,
			AssetCode: balance.Code,
			Issuer:    balance.Issuer,
			Balance:   balance.Balance,
		})
	}

	response := WalletDetailsResponse{
		PublicKey:      publicKey,
		Exists:         true,
		Balances:       balances,
		SequenceNumber: account.Sequence,
	}
	c.JSON(http.StatusOK, response)
}

func transferFundsHandler(c *gin.Context, config Config) {
	// Parse and validate request body
	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	// Validate secret key
	senderKP, err := keypair.ParseFull(req.FromSecretKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sender secret key"})
		return
	}

	// Validate public key
	if _, err := keypair.ParseAddress(req.ToPublicKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid recipient public key"})
		return
	}

	// Validate amount
	if amountFloat, err := strconv.ParseFloat(req.Amount, 64); err != nil || amountFloat <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount: must be a positive number"})
		return
	}

	// Fetch sender account details
	accountRequest := horizonclient.AccountRequest{AccountID: senderKP.Address()}
	sourceAccount, err := config.HorizonClient.AccountDetail(accountRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sender account details: " + err.Error()})
		return
	}

	// Build payment operation for USDC
	paymentOp := txnbuild.Payment{
		Destination: req.ToPublicKey,
		Amount:      req.Amount,
		Asset:       config.USDCAsset,
	}

	// Build transaction
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			Operations:           []txnbuild.Operation{&paymentOp},
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
			IncrementSequenceNum: true,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build transaction: " + err.Error()})
		return
	}

	// Sign transaction
	var networkPassphrase string
	if config.Network == "testnet" {
		networkPassphrase = network.TestNetworkPassphrase
	} else {
		networkPassphrase = network.PublicNetworkPassphrase
	}

	tx, err = tx.Sign(networkPassphrase, senderKP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign transaction: " + err.Error()})
		return
	}

	// Submit transaction
	resp, err := config.HorizonClient.SubmitTransaction(tx)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed", "details": herr.Problem.Detail})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction: " + err.Error()})
		return
	}

	// Success response
	response := TransferResponse{
		TransactionHash: resp.Hash,
		Message:         "USDC transferred successfully",
	}
	c.JSON(http.StatusOK, response)
}
