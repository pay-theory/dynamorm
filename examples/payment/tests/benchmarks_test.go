package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/example/dynamorm"
	"github.com/example/dynamorm/examples/payment"
	"github.com/example/dynamorm/examples/payment/utils"
)

// BenchmarkPaymentCreate benchmarks single payment creation
func BenchmarkPaymentCreate(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			payment := &payment.Payment{
				ID:             uuid.New().String(),
				IdempotencyKey: uuid.New().String(),
				MerchantID:     merchant.ID,
				Amount:         1000,
				Currency:       "USD",
				Status:         payment.PaymentStatusPending,
				PaymentMethod:  "card",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				Version:        1,
			}

			if err := db.Model(payment).Create(); err != nil {
				b.Error(err)
			}
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "payments/sec")
}

// BenchmarkIdempotencyCheck benchmarks idempotency key checking
func BenchmarkIdempotencyCheck(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)

	// Pre-populate some idempotency records
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		_, _ = idempotency.Process(nil, merchant.ID, key, func() (interface{}, error) {
			return &payment.Payment{ID: fmt.Sprintf("payment-%d", i)}, nil
		})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of existing and new keys
			key := fmt.Sprintf("bench-key-%d", i%1500)
			_, _ = idempotency.Process(nil, merchant.ID, key, func() (interface{}, error) {
				return &payment.Payment{ID: fmt.Sprintf("payment-new-%d", i)}, nil
			})
			i++
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "checks/sec")
}

// BenchmarkBatchPaymentCreate benchmarks batch payment creation
func BenchmarkBatchPaymentCreate(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	batchSizes := []int{10, 25, 50, 100}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("BatchSize-%d", batchSize), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				payments := make([]*payment.Payment, batchSize)
				for j := 0; j < batchSize; j++ {
					payments[j] = &payment.Payment{
						ID:             fmt.Sprintf("batch-%d-%d", i, j),
						IdempotencyKey: fmt.Sprintf("batch-key-%d-%d", i, j),
						MerchantID:     merchant.ID,
						Amount:         int64(1000 + j),
						Currency:       "USD",
						Status:         payment.PaymentStatusSucceeded,
						PaymentMethod:  "card",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Version:        1,
					}
				}

				// Batch create
				if err := db.Model(&payment.Payment{}).BatchCreate(payments); err != nil {
					b.Error(err)
				}
			}

			totalPayments := b.N * batchSize
			b.ReportMetric(float64(totalPayments)/b.Elapsed().Seconds(), "payments/sec")
			b.ReportMetric(b.Elapsed().Seconds()/float64(b.N), "sec/batch")
		})
	}
}

// BenchmarkQueryMerchantPayments benchmarks querying payments by merchant
func BenchmarkQueryMerchantPayments(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)

	// Pre-populate payments
	for i := 0; i < 10000; i++ {
		payment := &payment.Payment{
			ID:             fmt.Sprintf("query-payment-%d", i),
			IdempotencyKey: fmt.Sprintf("query-key-%d", i),
			MerchantID:     merchant.ID,
			Amount:         int64(1000 + (i % 1000)),
			Currency:       "USD",
			Status:         getRandomStatus(i),
			PaymentMethod:  "card",
			CustomerID:     fmt.Sprintf("customer-%d", i%100),
			CreatedAt:      time.Now().Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      time.Now(),
			Version:        1,
		}

		if err := db.Model(payment).Create(); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.Run("AllPayments", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", merchant.ID).
				Limit(100).
				All(&payments)

			if err != nil {
				b.Error(err)
			}
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/sec")
	})

	b.Run("FilteredByStatus", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", merchant.ID).
				Where("Status", "=", payment.PaymentStatusSucceeded).
				Limit(100).
				All(&payments)

			if err != nil {
				b.Error(err)
			}
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/sec")
	})

	b.Run("WithPagination", func(b *testing.B) {
		var cursor string
		for i := 0; i < b.N; i++ {
			var payments []*payment.Payment
			query := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", merchant.ID).
				Limit(20)

			if cursor != "" {
				query = query.Cursor(cursor)
			}

			nextCursor, err := query.All(&payments)
			if err != nil {
				b.Error(err)
			}

			// Reset cursor after 5 pages
			if i%5 == 0 {
				cursor = ""
			} else {
				cursor = nextCursor
			}
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "pages/sec")
	})
}

// BenchmarkComplexTransaction benchmarks complex payment transactions
func BenchmarkComplexTransaction(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	customer := createBenchCustomer(b, db, merchant.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Start transaction
		tx := db.Transaction()

		// Create payment
		payment := &payment.Payment{
			ID:             fmt.Sprintf("tx-payment-%d", i),
			IdempotencyKey: fmt.Sprintf("tx-key-%d", i),
			MerchantID:     merchant.ID,
			CustomerID:     customer.ID,
			Amount:         5000,
			Currency:       "USD",
			Status:         payment.PaymentStatusPending,
			PaymentMethod:  "card",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			Version:        1,
		}

		if err := tx.Model(payment).Create(); err != nil {
			tx.Rollback()
			b.Error(err)
			continue
		}

		// Create transaction record
		transaction := &payment.Transaction{
			ID:          fmt.Sprintf("tx-trans-%d", i),
			PaymentID:   payment.ID,
			Type:        payment.TransactionTypeCapture,
			Amount:      payment.Amount,
			Status:      payment.PaymentStatusProcessing,
			ProcessedAt: time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Version:     1,
		}

		if err := tx.Model(transaction).Create(); err != nil {
			tx.Rollback()
			b.Error(err)
			continue
		}

		// Update payment status
		if err := tx.Model(&payment.Payment{}).
			Where("ID", "=", payment.ID).
			Update(map[string]interface{}{
				"Status":    payment.PaymentStatusSucceeded,
				"UpdatedAt": time.Now(),
			}); err != nil {
			tx.Rollback()
			b.Error(err)
			continue
		}

		// Commit
		if err := tx.Commit(); err != nil {
			b.Error(err)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "transactions/sec")
}

// BenchmarkConcurrentOperations benchmarks concurrent payment operations
func BenchmarkConcurrentOperations(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	concurrencyLevels := []int{1, 5, 10, 20, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency-%d", concurrency), func(b *testing.B) {
			b.ResetTimer()

			var wg sync.WaitGroup
			semaphore := make(chan struct{}, concurrency)

			startTime := time.Now()
			totalOps := b.N

			for i := 0; i < totalOps; i++ {
				wg.Add(1)
				semaphore <- struct{}{}

				go func(idx int) {
					defer wg.Done()
					defer func() { <-semaphore }()

					// Mix of operations
					switch idx % 4 {
					case 0: // Create
						payment := &payment.Payment{
							ID:             fmt.Sprintf("concurrent-%d", idx),
							IdempotencyKey: fmt.Sprintf("concurrent-key-%d", idx),
							MerchantID:     merchant.ID,
							Amount:         1000,
							Currency:       "USD",
							Status:         payment.PaymentStatusPending,
							PaymentMethod:  "card",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
							Version:        1,
						}
						_ = db.Model(payment).Create()

					case 1: // Query
						var payments []*payment.Payment
						_ = db.Model(&payment.Payment{}).
							Index("gsi-merchant").
							Where("MerchantID", "=", merchant.ID).
							Limit(10).
							All(&payments)

					case 2: // Update
						_ = db.Model(&payment.Payment{}).
							Where("ID", "=", fmt.Sprintf("concurrent-%d", idx-1)).
							Update(map[string]interface{}{
								"Status":    payment.PaymentStatusSucceeded,
								"UpdatedAt": time.Now(),
							})

					case 3: // Get
						var payment payment.Payment
						_ = db.Model(&payment.Payment{}).
							Where("ID", "=", fmt.Sprintf("concurrent-%d", idx-2)).
							First(&payment)
					}
				}(i)
			}

			wg.Wait()
			elapsed := time.Since(startTime)

			b.ReportMetric(float64(totalOps)/elapsed.Seconds(), "ops/sec")
			b.ReportMetric(float64(concurrency), "concurrency")
		})
	}
}

// Helper functions

func initBenchDB(b *testing.B) (*dynamorm.DB, error) {
	db, err := dynamorm.New(
		dynamorm.WithRegion("us-east-1"),
		dynamorm.WithEndpoint("http://localhost:8000"),
		dynamorm.WithConnectionPool(50), // Higher pool for benchmarks
	)
	if err != nil {
		return nil, err
	}

	// Register models
	models := []interface{}{
		&payment.Payment{},
		&payment.Transaction{},
		&payment.Customer{},
		&payment.Merchant{},
		&payment.IdempotencyRecord{},
		&utils.AuditLog{},
	}

	for _, model := range models {
		db.Model(model)
	}

	return db, nil
}

func createBenchMerchant(b *testing.B, db *dynamorm.DB) *payment.Merchant {
	merchant := &payment.Merchant{
		ID:       fmt.Sprintf("bench-merchant-%d", time.Now().UnixNano()),
		Name:     "Benchmark Merchant",
		Email:    "bench@example.com",
		Status:   "active",
		Features: []string{"payments", "refunds"},
		RateLimits: payment.RateLimits{
			PaymentsPerMinute: 1000,
			PaymentsPerDay:    100000,
			MaxPaymentAmount:  1000000,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	if err := db.Model(merchant).Create(); err != nil {
		b.Fatal(err)
	}

	return merchant
}

func createBenchCustomer(b *testing.B, db *dynamorm.DB, merchantID string) *payment.Customer {
	customer := &payment.Customer{
		ID:         uuid.New().String(),
		MerchantID: merchantID,
		Email:      "bench-customer@example.com",
		Name:       "Benchmark Customer",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Version:    1,
	}

	if err := db.Model(customer).Create(); err != nil {
		b.Fatal(err)
	}

	return customer
}

func getRandomStatus(i int) string {
	statuses := []string{
		payment.PaymentStatusPending,
		payment.PaymentStatusProcessing,
		payment.PaymentStatusSucceeded,
		payment.PaymentStatusFailed,
		payment.PaymentStatusCanceled,
	}
	return statuses[i%len(statuses)]
}
