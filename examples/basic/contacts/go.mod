module github.com/pay-theory/dynamorm/examples/basic/contacts

go 1.21

require (
	github.com/aws/aws-sdk-go-v2/config v1.29.15
	github.com/aws/aws-sdk-go-v2/credentials v1.17.68
	github.com/google/uuid v1.6.0
	github.com/pay-theory/dynamorm v0.0.0-00010101000000-000000000000
)

replace github.com/pay-theory/dynamorm => ../../.. 