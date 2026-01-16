package query

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
)

func TestUnmarshalItem_DynamORMTagSemantics_CON3(t *testing.T) {
	type model struct {
		_ struct{} `dynamorm:"naming:snake_case"`

		ID        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
		UserID    string
		CreatedAt time.Time `dynamorm:"created_at"`
		Custom    string    `dynamorm:"attr:custom_name"`
	}

	item := map[string]types.AttributeValue{
		"id":          &types.AttributeValueMemberS{Value: "p1"},
		"sk":          &types.AttributeValueMemberS{Value: "s1"},
		"user_id":     &types.AttributeValueMemberS{Value: "u1"},
		"created_at":  &types.AttributeValueMemberS{Value: "2020-01-01T00:00:00Z"},
		"custom_name": &types.AttributeValueMemberS{Value: "c"},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "p1", out.ID)
	require.Equal(t, "s1", out.SK)
	require.Equal(t, "u1", out.UserID)
	require.Equal(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), out.CreatedAt)
	require.Equal(t, "c", out.Custom)
}

func TestUnmarshalItem_EncryptedEnvelope_FailsClosed_CON3(t *testing.T) {
	type model struct {
		_ struct{} `dynamorm:"naming:snake_case"`

		Secret string `dynamorm:"encrypted,attr:secret"`
	}

	item := map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"v":     &types.AttributeValueMemberN{Value: "1"},
			"edk":   &types.AttributeValueMemberB{Value: []byte("edk")},
			"nonce": &types.AttributeValueMemberB{Value: []byte("nonce")},
			"ct":    &types.AttributeValueMemberB{Value: []byte("ct")},
		}},
	}

	var out model
	err := UnmarshalItem(item, &out)
	require.Error(t, err)
	require.True(t, errors.Is(err, customerrors.ErrEncryptionNotConfigured))
}

func TestUnmarshalItem_EncryptedTag_AllowsNonEnvelopeValue_CON3(t *testing.T) {
	type model struct {
		_ struct{} `dynamorm:"naming:snake_case"`

		Secret string `dynamorm:"encrypted,attr:secret"`
	}

	item := map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberS{Value: "plaintext"},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "plaintext", out.Secret)
}
