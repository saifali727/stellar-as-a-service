package services

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/saif727/stellar-wallet-backend/models"
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
	USDCAsset     txnbuild.CreditAsset
}

// WalletService provides methods for wallet operations
type WalletService struct {
	Config Config
}

// NewWalletService creates a new WalletService instance
func NewWalletService(config Config) *WalletService {
	return &WalletService{Config: config}
}

// CreateWallet creates a new Stellar wallet and funds it with USDC
func (s *WalletService) CreateWallet() (*models.WalletResponse, error) {
	kp, err := keypair.Random()
	if err != nil {
		return nil, errors.New("failed to generate keypair: " + err.Error())
	}

	publicKey := kp.Address()
	secretKey := kp.Seed()

	masterKP, err := keypair.Parse(s.Config.MasterSecret)
	if err != nil {
		return nil, errors.New("invalid master secret key: " + err.Error())
	}

	createAccountOp := txnbuild.CreateAccount{
		Destination: publicKey,
		Amount:      "0.5",
	}

	usdcChangeTrustAsset, err := s.Config.USDCAsset.ToChangeTrustAsset()
	if err != nil {
		return nil, errors.New("failed to create USDC trustline asset: " + err.Error())
	}
	trustOp := txnbuild.ChangeTrust{
		Line: usdcChangeTrustAsset,
	}

	paymentOp := txnbuild.Payment{
		Destination: publicKey,
		Amount:      "100",
		Asset:       s.Config.USDCAsset,
	}

	accountRequest := horizonclient.AccountRequest{AccountID: masterKP.Address()}
	sourceAccount, err := s.Config.HorizonClient.AccountDetail(accountRequest)
	if err != nil {
		return nil, errors.New("failed to fetch master account details: " + err.Error())
	}

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
		return nil, errors.New("failed to build transaction: " + err.Error())
	}

	var networkPassphrase string
	if s.Config.Network == "testnet" {
		networkPassphrase = network.TestNetworkPassphrase
	} else {
		networkPassphrase = network.PublicNetworkPassphrase
	}

	masterFullKP, ok := masterKP.(*keypair.Full)
	if !ok {
		return nil, errors.New("master key is not a full keypair")
	}
	tx, err = tx.Sign(networkPassphrase, masterFullKP, kp)
	if err != nil {
		return nil, errors.New("failed to sign transaction: " + err.Error())
	}

	resp, err := s.Config.HorizonClient.SubmitTransaction(tx)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			return nil, errors.New("transaction failed: " + herr.Problem.Detail)
		}
		return nil, errors.New("failed to submit transaction: " + err.Error())
	}

	return &models.WalletResponse{
		PublicKey: publicKey,
		SecretKey: secretKey,
		Message:   "Wallet created, trusted USDC, and funded successfully. Hash: " + resp.Hash,
	}, nil
}

// GetWalletDetails retrieves details of a Stellar wallet
func (s *WalletService) GetWalletDetails(publicKey string) (*models.WalletDetailsResponse, error) {
	if _, err := keypair.ParseAddress(publicKey); err != nil {
		return nil, errors.New("invalid public key format")
	}

	accountRequest := horizonclient.AccountRequest{AccountID: publicKey}
	account, err := s.Config.HorizonClient.AccountDetail(accountRequest)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok && herr.Response.StatusCode == http.StatusNotFound {
			return &models.WalletDetailsResponse{
				PublicKey: publicKey,
				Exists:    false,
				Balances: []struct {
					AssetType string `json:"asset_type"`
					AssetCode string `json:"asset_code,omitempty"`
					Issuer    string `json:"issuer,omitempty"`
					Balance   string `json:"balance"`
				}{},
				SequenceNumber: 0,
			}, nil
		}
		return nil, errors.New("failed to fetch wallet details: " + err.Error())
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

	return &models.WalletDetailsResponse{
		PublicKey:      publicKey,
		Exists:         true,
		Balances:       balances,
		SequenceNumber: account.Sequence,
	}, nil
}

// TransferFunds transfers USDC between wallets
func (s *WalletService) TransferFunds(req models.TransferRequest) (*models.TransferResponse, error) {
	senderKP, err := keypair.ParseFull(req.FromSecretKey)
	if err != nil {
		return nil, errors.New("invalid sender secret key")
	}

	if _, err := keypair.ParseAddress(req.ToPublicKey); err != nil {
		return nil, errors.New("invalid recipient public key")
	}

	if amountFloat, err := strconv.ParseFloat(req.Amount, 64); err != nil || amountFloat <= 0 {
		return nil, errors.New("invalid amount: must be a positive number")
	}

	accountRequest := horizonclient.AccountRequest{AccountID: senderKP.Address()}
	sourceAccount, err := s.Config.HorizonClient.AccountDetail(accountRequest)
	if err != nil {
		return nil, errors.New("failed to fetch sender account details: " + err.Error())
	}

	paymentOp := txnbuild.Payment{
		Destination: req.ToPublicKey,
		Amount:      req.Amount,
		Asset:       s.Config.USDCAsset,
	}

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
		return nil, errors.New("failed to build transaction: " + err.Error())
	}

	var networkPassphrase string
	if s.Config.Network == "testnet" {
		networkPassphrase = network.TestNetworkPassphrase
	} else {
		networkPassphrase = network.PublicNetworkPassphrase
	}

	tx, err = tx.Sign(networkPassphrase, senderKP)
	if err != nil {
		return nil, errors.New("failed to sign transaction: " + err.Error())
	}

	resp, err := s.Config.HorizonClient.SubmitTransaction(tx)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			return nil, errors.New("transaction failed: " + herr.Problem.Detail)
		}
		return nil, errors.New("failed to submit transaction: " + err.Error())
	}

	return &models.TransferResponse{
		TransactionHash: resp.Hash,
		Message:         "USDC transferred successfully",
	}, nil
}
