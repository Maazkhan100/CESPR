package solar_recycling_chaincode

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SolarRecyclingContract defines the chaincode structure
type SolarRecyclingContract struct {
	contractapi.Contract
}

// Threshold for energy units required before contributing recycling cost
const thresholdUnits = 500.0

// SolarPanel represents the lifecycle of a solar panel
type SolarPanel struct {
	ID                 string         `json:"id"`
	Manufacturer       string         `json:"manufacturer"`
	Prosumer           string         `json:"prosumer"`
	RecyclingCompany   string         `json:"recyclingCompany"`
	RecyclingCost      float64        `json:"recyclingCost"`
	WarrantyClaim      string         `json:"warrantyClaim"`
	TotalContributions float64        `json:"totalContributions"`
	Status             string         `json:"status"` // "active", "recycled", "failed"
	PurchaseDate       string         `json:"purchaseDate"`
	EOLDate            string         `json:"eolDate"`
	FailureDate        string         `json:"failureDate,omitempty"`
	EnergyGenerated    float64        `json:"energyGenerated"`
	Contributions      []Contribution `json:"contributions"`
	Acknowledgments    []string       `json:"acknowledgments"`
}

// Contribution represents the recycling cost contributions
type Contribution struct {
	Prosumer         string  `json:"prosumer"`
	Amount           float64 `json:"amount"`
	ContributionDate string  `json:"contributionDate"`
}

// Agreement struct stores the agreement between a prosumer and the manufacturer
type Agreement struct {
	ID               string  `json:"id"`
	Manufacturer     string  `json:"manufacturer"`
	Prosumer         string  `json:"prosumer"`
	RecyclingCompany string  `json:"recyclingCompany"`
	RecyclingCost    float64 `json:"recyclingCost"`
	WarrantyClaim    string  `json:"warrantyClaim"`
	PurchaseDate     string  `json:"purchaseDate"`
	EndOfLifeDate    string  `json:"endOfLifeDate"`
	SolarPanelID     string  `json:"solarPanelId"`
}

// EscrowAccount represents an escrow account with balance and transaction history
// One EscrowAccount as only Utility stakeholder is allowed to maintain it
type EscrowAccount struct {
	AccountID    string        `json:"accountID"`
	Balance      float64       `json:"balance"`
	LastUpdated  string        `json:"lastUpdated"`
	Transactions []Transaction `json:"transactions"`
}

// Transaction represents a financial transaction in the escrow account
type Transaction struct {
	TransactionID string  `json:"transactionID"`
	Type          string  `json:"type"` // "credit" or "debit"
	Amount        float64 `json:"amount"`
	Timestamp     string  `json:"timestamp"`
	Description   string  `json:"description"`
	ProsumerID    string  `json:"prosumerID"`
}

type AcknowledgmentTransaction struct {
	TransactionID string `json:"transaction_id"`
	PanelID       string `json:"panel_id"`
	Type          string `json:"type"`
	Timestamp     string `json:"timestamp"`
	Description   string `json:"description"`
	ProsumerID    string `json:"prosumer_id"`
}

// Solar panel fails before warranty period
type PanelTransactionBeforeWarranty struct {
	TransactionID          string  `json:"transactionID"`
	PanelID                string  `json:"panelID"`
	FailureDate            string  `json:"failureDate"`
	Cause                  string  `json:"cause"`
	RemainingRecyclingCost float64 `json:"remainingRecyclingCost"`
	WarrantyClaim          string  `json:"warrantyClaim"`
	Manufacturer           string  `json:"manufacturer"`
	ProsumerID             string  `json:"prosumerID"`
	Timestamp              string  `json:"timestamp"`
}

// PaymentTransactionManufacturer represents a payment transaction made by the manufacturer.
type PaymentTransactionManufacturer struct {
	TransactionID          string  `json:"transactionID"`
	ManufacturerID         string  `json:"manufacturerID"`
	PanelID                string  `json:"panelID"`
	AmountDeposited        float64 `json:"amountDeposited"`
	RemainingRecyclingCost float64 `json:"remainingRecyclingCost"`
	BankAccountID          string  `json:"bankAccountID"`
	Timestamp              string  `json:"timestamp"`
}

// InitLedger initializes the ledger with some initial data
func (s *SolarRecyclingContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
	solarPanels := []SolarPanel{
		{ID: "SP001", Manufacturer: "MFG1", Prosumer: "P001", RecyclingCost: 45.0, Status: "active", PurchaseDate: "2023-01-01", EOLDate: "2048-01-01", EnergyGenerated:},
	}

	for _, panel := range solarPanels {
		panelJSON, err := json.Marshal(panel)
		if err != nil {
			return err
		}
		err = ctx.GetStub().PutState(panel.ID, panelJSON)
		if err != nil {
			return fmt.Errorf("failed to add solar panel %s: %v", panel.ID, err)
		}
	}

	return nil
}

// Step: 0
// CreateAgreement creates an agreement between the prosumer and the manufacturer
// Solar Panel registration (step 1) is called in this function
func (s *SolarRecyclingContract) CreateAgreement(ctx contractapi.TransactionContextInterface, agreementID, manufacturer, prosumer, recyclingCompany, solarPanelID, purchaseDate, eolDate, warrantyClaim string, recyclingCost float64) error {
	exists, err := s.AgreementExists(ctx, agreementID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("agreement %s already exists", agreementID)
	}

	agreement := Agreement{
		ID:               agreementID,
		Manufacturer:     manufacturer,
		Prosumer:         prosumer,
		RecyclingCompany: recyclingCompany,
		RecyclingCost:    recyclingCost,
		WarrantyClaim:    warrantyClaim,
		PurchaseDate:     purchaseDate,
		EndOfLifeDate:    eolDate,
		SolarPanelID:     solarPanelID,
	}

	agreementJSON, err := json.Marshal(agreement)
	if err != nil {
		return err
	}

	// Store the agreement in the ledger
	err = ctx.GetStub().PutState(agreementID, agreementJSON)
	if err != nil {
		return fmt.Errorf("failed to store agreement: %v", err)
	}

	// **Step 1**: Register the solar panel immediately after creating the agreement
	return s.RegisterSolarPanel(ctx, solarPanelID, manufacturer, prosumer, recyclingCompany, recyclingCost, warrantyClaim, purchaseDate, eolDate)
}

// GetAgreement retrieves the details of an agreement by its ID
func (s *SolarRecyclingContract) GetAgreement(ctx contractapi.TransactionContextInterface, agreementID string) (*Agreement, error) {
	agreementJSON, err := ctx.GetStub().GetState(agreementID)
	if err != nil {
		return nil, fmt.Errorf("failed to read agreement %s: %v", agreementID, err)
	}
	if agreementJSON == nil {
		return nil, fmt.Errorf("agreement %s does not exist", agreementID)
	}

	var agreement Agreement
	err = json.Unmarshal(agreementJSON, &agreement)
	if err != nil {
		return nil, err
	}

	return &agreement, nil
}

// AgreementExists checks if an agreement exists in the ledger
func (s *SolarRecyclingContract) AgreementExists(ctx contractapi.TransactionContextInterface, agreementID string) (bool, error) {
	agreementJSON, err := ctx.GetStub().GetState(agreementID)
	if err != nil {
		return false, err
	}

	return agreementJSON != nil, nil
}

// Step 1.
// RegisterSolarPanel registers a new solar panel
func (s *SolarRecyclingContract) RegisterSolarPanel(ctx contractapi.TransactionContextInterface, id, manufacturer, prosumer string, recyclingCost float64, warrantyClaim, purchaseDate, eolDate string) error {
	exists, err := s.SolarPanelExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("solar panel %s already exists", id)
	}

	panel := SolarPanel{
		ID:                 id,
		Manufacturer:       manufacturer,
		Prosumer:           prosumer,
		RecyclingCompany:   recyclingCompany,
		RecyclingCost:      recyclingCost,
		WarrantyClaim:      warrantyClaim,
		Status:             "active",
		PurchaseDate:       purchaseDate,
		EOLDate:            eolDate,
		EnergyGenerated:    0.0,
		TotalContributions: 0.0,
		Contributions:      []Contribution{},
		Acknowledgments:    []string{},
	}

	panelJSON, err := json.Marshal(panel)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(id, panelJSON)
}

// Step 2:
// RecordEnergyGeneration updates the energy generated by a solar panel
func (s *SolarRecyclingContract) RecordEnergyGeneration(ctx contractapi.TransactionContextInterface, panelID string, energyUnits float64) error {
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	// Update the energy generated
	panel.EnergyGenerated += energyUnits

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(panelID, updatedPanelJSON)
}

// Step 2 and 3:
// I think prosumer.
// ContributeRecyclingCost allows the prosumer to contribute towards recycling cost
func (s *SolarRecyclingContract) ContributeRecyclingCost(ctx contractapi.TransactionContextInterface, panelID string, amount float64) error {
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	// Check if the energy generated meets the threshold
	if panel.EnergyGenerated < thresholdUnits {
		return fmt.Errorf("insufficient energy generated")
	}

	// Create a contribution record
	contribution := Contribution{
		Prosumer:         panel.Prosumer,
		Amount:           amount,
		ContributionDate: "2006-01-02",
	}

	// Update the escrow account with the contribution
	escrowAccountID := "escrow_account" // A fixed escrow account ID used by Utility
	transactionID := fmt.Sprintf("TX_%s_%s", panelID, contribution.ContributionDate)
	description := fmt.Sprintf("Recycling contribution for panel %s", panelID)

	err = s.AddTransaction(ctx, escrowAccountID, transactionID, "credit", amount, contribution.ContributionDate, description, panel.Prosumer)
	if err != nil {
		return fmt.Errorf("failed to add contribution to escrow account: %v", err)
	}

	// Reset energy generated after contribution
	panel.EnergyGenerated = 0

	// Appending the transaction against the solar panel
	panel.Contributions = append(panel.Contributions, contribution)

	// Update the total contributions against the solar panel
	panel.TotalContributions += amount

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(panelID, updatedPanelJSON)
}

// Step 4:
func (s *SolarRecyclingContract) CreatePeriodicSummary(ctx contractapi.TransactionContextInterface, accountID string, summaryID string, currentTimestamp string) error {
	// Retrieve the escrow account details
	accountJSON, err := ctx.GetStub().GetState(accountID)
	if err != nil {
		return fmt.Errorf("failed to get account %s: %v", accountID, err)
	}
	if accountJSON == nil {
		return fmt.Errorf("escrow account %s does not exist", accountID)
	}

	var account EscrowAccount
	err = json.Unmarshal(accountJSON, &account)
	if err != nil {
		return err
	}

	// Extract unique active customers from transaction history
	customerList := make([]string, 0)
	activeCustomers := make(map[string]bool)

	for _, transaction := range account.Transactions {
		if transaction.Type == "credit" { // Only consider "credit" transactions
			if !activeCustomers[transaction.ProsumerID] {
				activeCustomers[transaction.ProsumerID] = true
				customerList = append(customerList, transaction.ProsumerID)
			}
		}
	}

	// Create a summary of the account's balance and transactions
	summary := map[string]interface{}{
		"summaryID":    summaryID,
		"accountID":    account.AccountID,
		"balance":      account.Balance,
		"transactions": account.Transactions,
		"lastUpdated":  currentTimestamp,
	}

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return err
	}

	// Store the summary on the blockchain with a unique key
	summaryKey := fmt.Sprintf("summary_%s", summaryID)
	err = ctx.GetStub().PutState(summaryKey, summaryJSON)
	if err != nil {
		return fmt.Errorf("failed to store summary %s: %v", summaryID, err)
	}

	// Clear the transaction history after creating the summary
	account.Transactions = []Transaction{}
	account.LastUpdated = currentTimestamp

	// Save the updated escrow account back to the ledger
	updatedAccountJSON, err := json.Marshal(account)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(accountID, updatedAccountJSON)
}

func (s *SolarRecyclingContract) AddTransaction(ctx contractapi.TransactionContextInterface, accountID string, transactionID string, transactionType string, amount float64, timestamp string, description string, prosumerID string) error {
	// Retrieve the escrow account details
	accountJSON, err := ctx.GetStub().GetState(accountID)
	if err != nil {
		return fmt.Errorf("failed to get account %s: %v", accountID, err)
	}
	if accountJSON == nil {
		return fmt.Errorf("escrow account %s does not exist", accountID)
	}

	var account EscrowAccount
	err = json.Unmarshal(accountJSON, &account)
	if err != nil {
		return err
	}

	// Create a new transaction
	newTransaction := Transaction{
		TransactionID: transactionID,
		Type:          transactionType,
		Amount:        amount,
		Timestamp:     timestamp,
		Description:   description,
		ProsumerID:    prosumerID,
	}

	// Update the account balance based on the transaction type
	if transactionType == "credit" {
		account.Balance += amount
	} else if transactionType == "debit" {
		if account.Balance < amount {
			return errors.New("insufficient funds in escrow account")
		}
		account.Balance -= amount
	} else {
		return errors.New("invalid transaction type; must be 'credit' or 'debit'")
	}

	// Add the transaction to the account's transaction history
	account.Transactions = append(account.Transactions, newTransaction)

	// Save the updated escrow account back to the ledger
	updatedAccountJSON, err := json.Marshal(account)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(accountID, updatedAccountJSON)
}

// Step 5
// Function to make payments to Prosumer and Recycler
func (s *SolarRecyclingContract) PayProsumerAndRecycler(ctx contractapi.TransactionContextInterface, panelID, escrowAccountID, recyclerID string) error {
	// Retrieve the solar panel details
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	// Check if the panel has reached its EOL
	if panel.Status != "active" {
		return fmt.Errorf("panel %s is not active or already processed", panelID)
	}

	// Retrieve the escrow account details
	escrowJSON, err := ctx.GetStub().GetState(escrowAccountID)
	if err != nil {
		return fmt.Errorf("failed to read escrow account %s: %v", escrowAccountID, err)
	}
	if escrowJSON == nil {
		return fmt.Errorf("escrow account %s does not exist", escrowAccountID)
	}

	var escrowAccount EscrowAccount
	err = json.Unmarshal(escrowJSON, &escrowAccount)
	if err != nil {
		return err
	}

	// Ensure there's sufficient balance in the escrow account
	if escrowAccount.Balance < panel.TotalContributions {
		return fmt.Errorf("insufficient balance in escrow account to make payments")
	}

	// Calculate payments
	recyclerPayment := panel.RecyclingCost
	prosumerPayment := panel.TotalContributions - recyclerPayment

	// Ensure contributions cover the panel transfer cost
	if prosumerPayment <= 0 {
		return fmt.Errorf("insufficient contributions to cover the panel transferring cost")
	}

	currentTimestamp := "current_date_placeholder" // Replace with actual timestamp logic

	// Record the payment to the recycler
	err = s.AddTransaction(ctx, escrowAccount.AccountID, fmt.Sprintf("TX_REC_%s", panelID), "debit", recyclerPayment, currentTimestamp, "Payment to recycler", recyclerID)
	if err != nil {
		return fmt.Errorf("failed to pay recycler: %v", err)
	}

	// Record the payment to the single prosumer
	err = s.AddTransaction(ctx, escrowAccount.AccountID, fmt.Sprintf("TX_PRO_%s", panelID), "debit", prosumerPayment, currentTimestamp, "Payment to prosumer", panel.Prosumer)
	if err != nil {
		return fmt.Errorf("failed to pay prosumer %s: %v", panel.Prosumer, err)
	}

	// Update the solar panel status to "recycled"
	panel.Status = "recycled"
	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return err
	}
	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel status: %v", err)
	}

	return nil
}

// Step 6
// Acknowledgement transaction
func (s *SolarRecyclingContract) AcknowledgeTransportFee(ctx contractapi.TransactionContextInterface, panelID string, prosumerID string) error {
	// Retrieve the solar panel details
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return fmt.Errorf("failed to unmarshal panel data: %v", err)
	}

	// Ensure the prosumer ID matches the panel's registered prosumer
	if prosumerID != panel.Prosumer {
		return fmt.Errorf("provided prosumer ID does not match the registered prosumer for panel %s", panelID)
	}

	// Generate a random acknowledgment transaction ID
	transactionID := fmt.Sprintf("ACK_TRANSPORT_%s", panelID)

	// Create acknowledgment transaction
	acknowledgmentTransaction := AcknowledgmentTransaction{
		TransactionID: transactionID,
		PanelID:       panelID,
		Type:          "Transport Fee Acknowledgment",
		Timestamp:     "2006-01-02",
		Description:   fmt.Sprintf("Transport fee acknowledged by Prosumer: %s for Panel ID: %s", prosumerID, panelID),
		ProsumerID:    prosumerID,
	}

	// Save acknowledgment transaction as a separate record
	acknowledgmentJSON, err := json.Marshal(acknowledgmentTransaction)
	if err != nil {
		return fmt.Errorf("failed to marshal acknowledgment transaction: %v", err)
	}

	err = ctx.GetStub().PutState(transactionID, acknowledgmentJSON)
	if err != nil {
		return fmt.Errorf("failed to record acknowledgment transaction: %v", err)
	}

	// Update the panel's acknowledgment field
	panel.Acknowledgments = append(panel.Acknowledgments, transactionID)

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated panel: %v", err)
	}

	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel record with acknowledgment: %v", err)
	}

	return nil
}

// Step 7
func (s *SolarRecyclingContract) SendToRecycler(ctx contractapi.TransactionContextInterface, panelID string, recyclingCompany string) error {
	// Retrieve the solar panel record
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to retrieve panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return fmt.Errorf("failed to unmarshal panel data: %v", err)
	}

	// Update the panel's status to "Transferred to Recycler"
	panel.Status = "Transferred to Recycler"

	// Save the updated panel back to the ledger
	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated panel data: %v", err)
	}

	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel %s: %v", panelID, err)
	}

	return nil
}

// Step 8
func (s *SolarRecyclingContract) AcknowledgeReceiptEOLPanel(ctx contractapi.TransactionContextInterface, panelID string, recyclerID string) error {

	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to retrieve panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return fmt.Errorf("failed to unmarshal panel data: %v", err)
	}

	// Ensure the panel is in the correct status for acknowledgment
	if panel.Status != "Transferred to Recycler" {
		return fmt.Errorf("panel %s has not been transferred to a recycler", panelID)
	}

	// Create an acknowledgment transaction
	acknowledgmentTransaction := AcknowledgmentTransaction{
		TransactionID: fmt.Sprintf("ACK_RECEIPT_%s", panelID),
		PanelID:       panelID,
		Type:          "Receipt Acknowledgment",
		Timestamp:     ctx.GetStub().GetTxTimestamp().String(),
		Description:   fmt.Sprintf("Recycler %s acknowledged receipt of panel %s", recyclerID, panelID),
		ProsumerID:    panel.Prosumer,
	}

	// Marshal and save the acknowledgment transaction
	acknowledgmentJSON, err := json.Marshal(acknowledgmentTransaction)
	if err != nil {
		return fmt.Errorf("failed to marshal acknowledgment transaction: %v", err)
	}

	acknowledgmentKey := fmt.Sprintf("ACKNOWLEDGMENT_%s", panelID)
	err = ctx.GetStub().PutState(acknowledgmentKey, acknowledgmentJSON)
	if err != nil {
		return fmt.Errorf("failed to save acknowledgment transaction: %v", err)
	}

	// Update the panel's status to "Received by Recycler"
	panel.Status = "Received by Recycler"

	// Add acknowledgment to the panel's acknowledgment list
	panel.Acknowledgments = append(panel.Acknowledgments, acknowledgmentTransaction.TransactionID)

	// Save the updated panel back to the ledger
	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated panel data: %v", err)
	}

	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel %s: %v", panelID, err)
	}

	return nil
}

// Step 10
func (s *SolarRecyclingContract) AcknowledgeReceiptOfPayment(ctx contractapi.TransactionContextInterface, panelID string, recyclerID string) error {
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to retrieve panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return fmt.Errorf("failed to unmarshal panel data: %v", err)
	}

	if panel.Status != "Received by Recycler" {
		return fmt.Errorf("panel %s has not been received by the recycler yet", panelID)
	}

	acknowledgmentTransaction := AcknowledgmentTransaction{
		TransactionID: fmt.Sprintf("ACK_PAYMENT_%s", panelID),
		PanelID:       panelID,
		Type:          "Payment Receipt Acknowledgment by Recycler",
		Timestamp:     "30-12-2024",
		Description:   fmt.Sprintf("Recycler %s acknowledged receipt of payment for panel %s", recyclerID, panelID),
		ProsumerID:    panel.Prosumer,
	}

	acknowledgmentJSON, err := json.Marshal(acknowledgmentTransaction)
	if err != nil {
		return fmt.Errorf("failed to marshal acknowledgment transaction: %v", err)
	}

	acknowledgmentKey := fmt.Sprintf("ACKNOWLEDGMENT_PAYMENT_%s", panelID)
	err = ctx.GetStub().PutState(acknowledgmentKey, acknowledgmentJSON)
	if err != nil {
		return fmt.Errorf("failed to save acknowledgment transaction: %v", err)
	}

	panel.Acknowledgments = append(panel.Acknowledgments, acknowledgmentTransaction.TransactionID)

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated panel data: %v", err)
	}

	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel %s: %v", panelID, err)
	}

	return nil
}

// Function when solar panel fails before warranty:
func (s *SolarRecyclingContract) RecordFailure(ctx contractapi.TransactionContextInterface, panelID, failureDate, cause, prosumerID string) error {
	// Retrieve the panel state
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	// Calculate remaining recycling cost
	remainingRecyclingCost := panel.RecyclingCost - panel.TotalContributions
	warrantyClaim := fmt.Sprintf("Warranty claim initiated for manufacturer: %s", panel.Manufacturer)

	// Create a new transaction record
	transactionID := "TX_" + panelID + "_" + failureDate
	transaction := PanelTransactionBeforeWarranty{
		TransactionID:          transactionID,
		PanelID:                panelID,
		FailureDate:            failureDate,
		Cause:                  cause,
		RemainingRecyclingCost: remainingRecyclingCost,
		WarrantyClaim:          warrantyClaim,
		Manufacturer:           panel.Manufacturer,
		ProsumerID:             prosumerID,
		Timestamp:              "time.Now().Format(time.RFC3339)",
	}

	transactionJSON, err := json.Marshal(transaction)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	// Log transaction on the blockchain ledger
	err = ctx.GetStub().PutState(transactionID, transactionJSON)
	if err != nil {
		return fmt.Errorf("failed to record transaction: %v", err)
	}

	// Update the panel state
	panel.FailureDate = failureDate
	panel.Status = "failed"
	acknowledgment := fmt.Sprintf("Failure recorded: Cause - %s, Remaining Recycling Cost - %.2f", cause, remainingRecyclingCost)
	panel.Acknowledgments = append(panel.Acknowledgments, acknowledgment)

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated panel: %v", err)
	}

	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel: %v", err)
	}

	return nil
}

func (s *SolarRecyclingContract) ManufacturerDeposit(ctx contractapi.TransactionContextInterface, manufacturerID, panelID string, amount, transportFee float64) error {
	escrowAccountID := "escrow_account" // Escrow account managed by Utility
	transactionDate := "2025-01-02"

	// Retrieve the panel details
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	// Check the panel's status
	if panel.Status != "failed" {
		return fmt.Errorf("panel %s is not eligible for manufacturer payment", panelID)
	}

	// Calculate remaining recycling cost
	remainingRecyclingCost := panel.RecyclingCost - panel.TotalContributions

	// Calculate the total amount owed
	totalAmountOwed := remainingRecyclingCost + transportFee

	// Validate the payment amount
	if amount < totalAmountOwed {
		return fmt.Errorf("amount is less than the total cost (recycling + transport) for panel %s. Expected: %.2f, Received: %.2f", panelID, totalAmountOwed, amount)
	}

	// Create a transaction ID
	transactionID := fmt.Sprintf("TX_%s_%s", panelID, transactionDate)

	// Add the transaction to the escrow account
	description := fmt.Sprintf("Manufacturer payment for recycling and transport of panel %s", panelID)
	err = s.AddTransaction(ctx, escrowAccountID, transactionID, "credit", amount, transactionDate, description, manufacturerID)
	if err != nil {
		return fmt.Errorf("failed to record payment in escrow account: %v", err)
	}

	// Update the panel's contribution details
	acknowledgment := fmt.Sprintf("Manufacturer %s deposited %.2f (including %.2f transport fee) on %s. Remaining cost: %.2f.", manufacturerID, amount, transportFee, transactionDate, totalAmountOwed-amount)
	panel.Acknowledgments = append(panel.Acknowledgments, acknowledgment)
	panel.TotalContributions += amount

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return err
	}

	// Save the updated panel
	err = ctx.GetStub().PutState(panelID, updatedPanelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel %s: %v", panelID, err)
	}

	return nil
}

// Code for when solar panel fails befoe the EOL and after warranty period:
// Function to handle panel failure by the prosumer
func (s *SolarRecyclingContract) ReportFailureByProsumer(ctx contractapi.TransactionContextInterface, panelID, failureDate, cause string) error {
	escrowAccountID := "escrow_account" // Escrow account managed by Utility
	transactionDate := "2006-01-02"

	// Retrieve the panel details
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	remainingRecyclingCost := panel.RecyclingCost - panel.TotalContributions
	if remainingRecyclingCost <= 0 {
		return fmt.Errorf("no remaining recycling cost for panel %s", panelID)
	}
	sharePerParty := remainingRecyclingCost / 3

	// Update panel's failure details
	panel.FailureDate = failureDate
	panel.Status = "failed"
	panel.Acknowledgments = append(panel.Acknowledgments, fmt.Sprintf("Panel failed after warranty on %s. Cause: %s.", failureDate, cause))

	// Prosumer's payment
	transactionID := fmt.Sprintf("TX_prosumer_%s_%s", panelID, transactionDate)
	description := fmt.Sprintf("Prosumer contribution for panel %s", panelID)

	err = s.AddTransaction(ctx, escrowAccountID, transactionID, "credit", sharePerParty, transactionDate, description, panel.Prosumer)
	if err != nil {
		return fmt.Errorf("failed to log prosumer contribution: %v", err)
	}

	// Append contribution acknowledgment to the panel
	acknowledgment := fmt.Sprintf("Prosumer contributed %.2f on %s.", sharePerParty, transactionDate)
	panel.Acknowledgments = append(panel.Acknowledgments, acknowledgment)

	panel.TotalContributions += sharePerParty

	// Save the updated panel
	panelJSON, err = json.Marshal(panel)
	if err != nil {
		return err
	}
	err = ctx.GetStub().PutState(panelID, panelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel %s: %v", panelID, err)
	}

	return nil
}

// Function to handle contributions from any stakeholder
func (s *SolarRecyclingContract) HandleContribution(ctx contractapi.TransactionContextInterface, panelID, contributorID, contributorType string) error {
	escrowAccountID := "escrow_account" // Escrow account managed by Utility
	transactionDate := "2006-01-02"

	// Retrieve the panel details
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return fmt.Errorf("failed to unmarshal panel %s: %v", panelID, err)
	}

	// Calculate the remaining recycling cost
	remainingRecyclingCost := panel.RecyclingCost - panel.TotalContributions
	if remainingRecyclingCost <= 0 {
		return fmt.Errorf("no remaining recycling cost for panel %s", panelID)
	}

	// Divide the remaining cost evenly among three contributors
	sharePerParty := panel.RecyclingCost / 2

	// Create a transaction for the contributor's payment
	transactionID := fmt.Sprintf("TX_%s_%s_%s", contributorType, panelID, transactionDate)
	description := fmt.Sprintf("%s contribution for panel %s", contributorType, panelID)

	err = s.AddTransaction(ctx, escrowAccountID, transactionID, "credit", sharePerParty, transactionDate, description, contributorID)
	if err != nil {
		return fmt.Errorf("failed to log %s contribution: %v", contributorType, err)
	}

	// Append contribution acknowledgment to the panel
	acknowledgment := fmt.Sprintf("%s contributed %.2f on %s.", contributorType, sharePerParty, transactionDate)
	panel.Acknowledgments = append(panel.Acknowledgments, acknowledgment)

	// Update the panel's total contributions
	panel.TotalContributions += sharePerParty

	// Save the updated panel back to the ledger
	panelJSON, err = json.Marshal(panel)
	if err != nil {
		return fmt.Errorf("failed to marshal updated panel %s: %v", panelID, err)
	}

	err = ctx.GetStub().PutState(panelID, panelJSON)
	if err != nil {
		return fmt.Errorf("failed to update panel %s: %v", panelID, err)
	}

	return nil
}

// SolarPanelExists checks if a solar panel exists
func (s *SolarRecyclingContract) SolarPanelExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	panelJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return false, err
	}

	return panelJSON != nil, nil
}

// MarkAsRecycled marks the solar panel as recycled and finalizes payments
func (s *SolarRecyclingContract) MarkAsRecycled(ctx contractapi.TransactionContextInterface, panelID string) error {
	panelJSON, err := ctx.GetStub().GetState(panelID)
	if err != nil {
		return fmt.Errorf("failed to read panel %s: %v", panelID, err)
	}
	if panelJSON == nil {
		return fmt.Errorf("panel %s does not exist", panelID)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return err
	}

	if panel.Status != "active" {
		return errors.New("panel is not in an active state")
	}

	panel.Status = "recycled"

	updatedPanelJSON, err := json.Marshal(panel)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(panelID, updatedPanelJSON)
}

// GetSolarPanel retrieves a solar panel's information
func (s *SolarRecyclingContract) GetSolarPanel(ctx contractapi.TransactionContextInterface, id string) (*SolarPanel, error) {
	panelJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return nil, fmt.Errorf("failed to read panel %s: %v", id, err)
	}
	if panelJSON == nil {
		return nil, fmt.Errorf("panel %s does not exist", id)
	}

	var panel SolarPanel
	err = json.Unmarshal(panelJSON, &panel)
	if err != nil {
		return nil, err
	}

	return &panel, nil
}
