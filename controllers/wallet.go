package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saif727/stellar-wallet-backend/models"
	"github.com/saif727/stellar-wallet-backend/services"
)

// WalletController handles wallet-related HTTP requests
type WalletController struct {
	Service *services.WalletService
}

// NewWalletController creates a new WalletController instance
func NewWalletController(service *services.WalletService) *WalletController {
	return &WalletController{Service: service}
}

// CreateWallet handles POST /api/v1/wallets/create
func (ctrl *WalletController) CreateWallet(c *gin.Context) {
	response, err := ctrl.Service.CreateWallet()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

// GetWalletDetails handles GET /api/v1/wallets/:public_key
func (ctrl *WalletController) GetWalletDetails(c *gin.Context) {
	publicKey := c.Param("public_key")
	response, err := ctrl.Service.GetWalletDetails(publicKey)
	if err != nil {
		if err.Error() == "invalid public key format" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, response)
}

// TransferFunds handles POST /api/v1/wallets/transfer
func (ctrl *WalletController) TransferFunds(c *gin.Context) {
	var req models.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	response, err := ctrl.Service.TransferFunds(req)
	if err != nil {
		if err.Error() == "invalid sender secret key" || err.Error() == "invalid recipient public key" || err.Error() == "invalid amount: must be a positive number" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, response)
}
