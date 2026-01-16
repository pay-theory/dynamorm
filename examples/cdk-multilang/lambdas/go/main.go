package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/pay-theory/dynamorm"
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/session"
)

type DemoItem struct {
	PK    string `dynamorm:"pk,attr:PK" json:"PK"`
	SK    string `dynamorm:"sk,attr:SK" json:"SK"`
	Value string `dynamorm:"attr:value" json:"value"`
	Lang  string `dynamorm:"attr:lang" json:"lang"`
}

func (DemoItem) TableName() string {
	return os.Getenv("TABLE_NAME")
}

type request struct {
	PK    string `json:"pk"`
	SK    string `json:"sk"`
	Value string `json:"value"`
}

func jsonResponse(status int, body any) (events.LambdaFunctionURLResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return events.LambdaFunctionURLResponse{StatusCode: http.StatusInternalServerError}, nil
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Headers:    map[string]string{"content-type": "application/json"},
		Body:       string(data),
	}, nil
}

func handler(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	db, err := dynamorm.New(session.Config{Region: os.Getenv("AWS_REGION")})
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	method := event.RequestContext.HTTP.Method
	if method == http.MethodGet {
		pk := event.QueryStringParameters["pk"]
		sk := event.QueryStringParameters["sk"]
		if pk == "" || sk == "" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "pk and sk are required"})
		}

		var out DemoItem
		err := db.Model(&DemoItem{PK: pk, SK: sk}).First(&out)
		if err != nil {
			if errors.Is(err, customerrors.ErrItemNotFound) {
				return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"})
			}
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "item": out})
	}

	var req request
	if event.Body != "" {
		_ = json.Unmarshal([]byte(event.Body), &req)
	}
	if req.PK == "" {
		req.PK = event.QueryStringParameters["pk"]
	}
	if req.SK == "" {
		req.SK = event.QueryStringParameters["sk"]
	}
	if req.Value == "" {
		req.Value = event.QueryStringParameters["value"]
	}
	if req.PK == "" || req.SK == "" {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "pk and sk are required"})
	}

	item := &DemoItem{
		PK:    req.PK,
		SK:    req.SK,
		Value: req.Value,
		Lang:  "go",
	}

	if err := db.Model(item).CreateOrUpdate(); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return jsonResponse(http.StatusOK, map[string]any{"ok": true, "item": item})
}

func main() {
	lambda.Start(handler)
}
