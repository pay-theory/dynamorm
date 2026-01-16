package encryption

import (
	"fmt"

	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
)

func MetadataHasEncryptedFields(metadata *model.Metadata) bool {
	if metadata == nil {
		return false
	}
	for _, fieldMeta := range metadata.Fields {
		if fieldMeta == nil {
			continue
		}
		if fieldMeta.IsEncrypted {
			return true
		}
		if _, ok := fieldMeta.Tags["encrypted"]; ok {
			return true
		}
	}
	return false
}

func FailClosedIfEncryptedWithoutKMSKeyARN(sess *session.Session, metadata *model.Metadata) error {
	if metadata == nil || !MetadataHasEncryptedFields(metadata) {
		return nil
	}

	keyARN := ""
	if sess != nil && sess.Config() != nil {
		keyARN = sess.Config().KMSKeyARN
	}
	if keyARN != "" {
		return nil
	}

	return fmt.Errorf("%w: model %s contains dynamorm:\"encrypted\" fields but session.Config.KMSKeyARN is empty", customerrors.ErrEncryptionNotConfigured, metadata.Type.Name())
}
