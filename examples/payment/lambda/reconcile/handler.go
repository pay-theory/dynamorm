package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/example/dynamorm"
	"github.com/example/dynamorm/examples/payment"
	"github.com/example/dynamorm/examples/payment/utils"
)

// ReconciliationRecord represents a row in the reconciliation CSV
type ReconciliationRecord struct {
	PaymentID      string
	TransactionID  string
	Status         string
	ProcessorFee   int64
	ProcessedDate  time.Time
	SettlementDate time.Time
}

// Handler processes reconciliation files
type Handler struct {
	db           *dynamorm.DB
	s3Client     *s3.S3
	auditTracker *utils.AuditTracker
}

// NewHandler creates a new reconciliation handler
func NewHandler() (*Handler, error) {
	// Initialize AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	// Initialize DynamoDB connection
	db, err := dynamorm.New(
		dynamorm.WithLambdaOptimization(),
		dynamorm.WithBatchSize(25), // DynamoDB batch limit
		dynamorm.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&payment.Payment{})
	db.Model(&payment.Transaction{})
	db.Model(&payment.Settlement{})

	return &Handler{
		db:           db,
		s3Client:     s3.New(sess),
		auditTracker: utils.NewAuditTracker(db),
	}, nil
}

// HandleRequest processes S3 events for reconciliation files
func (h *Handler) HandleRequest(ctx context.Context, event events.S3Event) error {
	for _, record := range event.Records {
		if err := h.processFile(ctx, record); err != nil {
			// Log error but continue processing other files
			fmt.Printf("Error processing file %s: %v\n", record.S3.Object.Key, err)
			continue
		}
	}
	return nil
}

// processFile processes a single reconciliation file
func (h *Handler) processFile(ctx context.Context, record events.S3EventRecord) error {
	bucket := record.S3.Bucket.Name
	key := record.S3.Object.Key

	// Download file from S3
	result, err := h.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer result.Body.Close()

	// Parse CSV
	reader := csv.NewReader(result.Body)

	// Skip header
	if _, err := reader.Read(); err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// Process in batches
	batch := make([]*ReconciliationRecord, 0, 100)
	batchCount := 0
	totalProcessed := 0
	totalErrors := 0

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			totalErrors++
			continue
		}

		// Parse row
		record, err := parseReconciliationRow(row)
		if err != nil {
			totalErrors++
			continue
		}

		batch = append(batch, record)

		// Process batch when full
		if len(batch) >= 100 {
			if err := h.processBatch(ctx, batch); err != nil {
				fmt.Printf("Error processing batch %d: %v\n", batchCount, err)
				totalErrors += len(batch)
			} else {
				totalProcessed += len(batch)
			}
			batch = batch[:0]
			batchCount++
		}
	}

	// Process remaining records
	if len(batch) > 0 {
		if err := h.processBatch(ctx, batch); err != nil {
			fmt.Printf("Error processing final batch: %v\n", err)
			totalErrors += len(batch)
		} else {
			totalProcessed += len(batch)
		}
	}

	// Create settlement summary
	settlement := &payment.Settlement{
		ID:               fmt.Sprintf("settlement-%s-%d", key, time.Now().Unix()),
		MerchantID:       extractMerchantFromKey(key),
		Date:             time.Now().Format("2006-01-02"),
		TransactionCount: totalProcessed,
		Status:           "completed",
		BatchID:          key,
		ProcessedAt:      time.Now(),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := h.db.Model(settlement).Create(); err != nil {
		return fmt.Errorf("failed to create settlement record: %w", err)
	}

	// Audit the reconciliation
	h.auditTracker.Track("reconciliation", "completed", map[string]interface{}{
		"file":       key,
		"processed":  totalProcessed,
		"errors":     totalErrors,
		"settlement": settlement.ID,
	})

	fmt.Printf("Reconciliation completed: %d processed, %d errors\n", totalProcessed, totalErrors)
	return nil
}

// processBatch processes a batch of reconciliation records
func (h *Handler) processBatch(ctx context.Context, records []*ReconciliationRecord) error {
	// Group by payment ID for efficient querying
	paymentGroups := make(map[string][]*ReconciliationRecord)
	for _, record := range records {
		paymentGroups[record.PaymentID] = append(paymentGroups[record.PaymentID], record)
	}

	// Process each payment group
	for paymentID, group := range paymentGroups {
		if err := h.reconcilePayment(ctx, paymentID, group); err != nil {
			fmt.Printf("Error reconciling payment %s: %v\n", paymentID, err)
			// Continue with other payments
		}
	}

	return nil
}

// reconcilePayment reconciles a single payment with its transactions
func (h *Handler) reconcilePayment(ctx context.Context, paymentID string, records []*ReconciliationRecord) error {
	// Start transaction
	tx := h.db.Transaction()
	defer tx.Rollback()

	// Get current payment
	var currentPayment payment.Payment
	if err := tx.Model(&payment.Payment{}).
		Where("ID", "=", paymentID).
		First(&currentPayment); err != nil {
		return fmt.Errorf("payment not found: %w", err)
	}

	// Update each transaction
	for _, record := range records {
		var transaction payment.Transaction
		if err := tx.Model(&payment.Transaction{}).
			Where("ID", "=", record.TransactionID).
			Where("PaymentID", "=", paymentID).
			First(&transaction); err != nil {
			continue // Skip if transaction not found
		}

		// Update transaction with reconciliation data
		updates := map[string]interface{}{
			"Status":      record.Status,
			"ProcessedAt": record.ProcessedDate,
			"UpdatedAt":   time.Now(),
		}

		// Add audit entry
		transaction.AuditTrail = append(transaction.AuditTrail, payment.AuditEntry{
			Timestamp: time.Now(),
			Action:    "reconciled",
			Changes: map[string]interface{}{
				"status":          record.Status,
				"processor_fee":   record.ProcessorFee,
				"settlement_date": record.SettlementDate,
			},
		})

		if err := tx.Model(&payment.Transaction{}).
			Where("ID", "=", transaction.ID).
			Update(updates); err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}
	}

	// Update payment status if needed
	if currentPayment.Status == payment.PaymentStatusPending {
		if err := tx.Model(&payment.Payment{}).
			Where("ID", "=", paymentID).
			Update(map[string]interface{}{
				"Status":    payment.PaymentStatusSucceeded,
				"UpdatedAt": time.Now(),
			}); err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}
	}

	// Commit transaction
	return tx.Commit()
}

// Helper functions

func parseReconciliationRow(row []string) (*ReconciliationRecord, error) {
	if len(row) < 6 {
		return nil, fmt.Errorf("invalid row format")
	}

	fee, err := strconv.ParseInt(row[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid fee: %w", err)
	}

	processedDate, err := time.Parse("2006-01-02", row[4])
	if err != nil {
		return nil, fmt.Errorf("invalid processed date: %w", err)
	}

	settlementDate, err := time.Parse("2006-01-02", row[5])
	if err != nil {
		return nil, fmt.Errorf("invalid settlement date: %w", err)
	}

	return &ReconciliationRecord{
		PaymentID:      row[0],
		TransactionID:  row[1],
		Status:         row[2],
		ProcessorFee:   fee,
		ProcessedDate:  processedDate,
		SettlementDate: settlementDate,
	}, nil
}

func extractMerchantFromKey(key string) string {
	// Extract merchant ID from S3 key
	// Format: reconciliation/merchant-123/2024-01-15.csv
	parts := strings.Split(key, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func main() {
	handler, err := NewHandler()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize handler: %v", err))
	}

	lambda.Start(handler.HandleRequest)
}
