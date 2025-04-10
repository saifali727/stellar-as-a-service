package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/saif727/stellar-wallet-backend/controllers"
	"github.com/saif727/stellar-wallet-backend/services"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/txnbuild"
)

func main() {
	// Load configuration from environment variables
	config := services.Config{
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

	// Initialize service and controller
	walletService := services.NewWalletService(config)
	walletController := controllers.NewWalletController(walletService)

	// Initialize Gin router
	router := gin.Default()

	// Define routes
	router.POST("/api/v1/wallets/create", walletController.CreateWallet)
	router.GET("/api/v1/wallets/:public_key", walletController.GetWalletDetails)
	router.POST("/api/v1/wallets/transfer", walletController.TransferFunds)

	// Run the server
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
