package transaction

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	stderrs "errors"
	"testing"
	"time"

	_ "unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"

	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

//go:linkname sessionConfigLoadFunc github.com/pay-theory/dynamorm/pkg/session.configLoadFunc
var sessionConfigLoadFunc func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)

func stubSessionConfigLoad(t *testing.T, fn func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)) {
	t.Helper()

	original := sessionConfigLoadFunc
	sessionConfigLoadFunc = fn

	t.Cleanup(func() {
		sessionConfigLoadFunc = original
	})
}

type stubHTTPClient struct {
	responses map[string]string
}

func (c stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	target := req.Header.Get("X-Amz-Target")
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}

	body := c.responses[target]
	if body == "" {
		body = "{}"
	}

	status := http.StatusOK
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewReader([]byte(body))),
		Request:       req,
	}, nil
}

func minimalAWSConfig(httpClient aws.HTTPClient) aws.Config {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "secret", "token"),
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
		HTTPClient: httpClient,
	}
	return cfg
}

type unitUser struct {
	UpdatedAt time.Time `dynamorm:"updated_at"`
	ID        string    `dynamorm:"pk"`
	Email     string
	Version   int `dynamorm:"version"`
}

func (unitUser) TableName() string {
	return "users_unit"
}

func TestTransaction_OperationsAndCommit(t *testing.T) {
	httpClient := stubHTTPClient{
		responses: map[string]string{
			"DynamoDB_20120810.TransactWriteItems": `{}`,
			"DynamoDB_20120810.TransactGetItems":  `{"Responses":[{"Item":{"id":{"S":"user-1"},"email":{"S":"test@example.com"}}}]}`,
		},
	}

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))
	converter := pkgTypes.NewConverter()

	tx := NewTransaction(sess, registry, converter)

	ctx := context.Background()
	require.Same(t, tx, tx.WithContext(ctx))
	require.Equal(t, ctx, tx.ctx)

	user := &unitUser{
		ID:      "user-1",
		Email:   "test@example.com",
		Version: 1,
	}

	require.NoError(t, tx.Create(user))
	require.NoError(t, tx.Update(user))
	require.NoError(t, tx.Delete(user))
	require.NoError(t, tx.Get(user, &unitUser{}))

	require.NoError(t, tx.Commit())
	require.NotEmpty(t, tx.results)
	require.Contains(t, tx.results, "0")
	require.NotNil(t, tx.results["0"])

	require.NoError(t, tx.Rollback())
	require.Nil(t, tx.writes)
	require.Nil(t, tx.reads)
	require.Nil(t, tx.results)
}

func TestTransaction_handleTransactionError(t *testing.T) {
	tx := &Transaction{}

	require.NoError(t, tx.handleTransactionError(nil))

	err := tx.handleTransactionError(stderrs.New("prefix ConditionalCheckFailed suffix"))
	require.ErrorIs(t, err, dynamormerrors.ErrConditionFailed)

	err = tx.handleTransactionError(stderrs.New("prefix TransactionCanceled suffix"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "transaction canceled")

	err = tx.handleTransactionError(stderrs.New("prefix ValidationException suffix"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation error")

	other := stderrs.New("something else")
	require.ErrorIs(t, tx.handleTransactionError(other), other)
}
