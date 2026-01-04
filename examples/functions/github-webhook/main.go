package githubwebhook
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type GitHubEvent struct {
	Action     string `json:"action"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`



















}	lambda.Start(handler)func main() {}	}, nil		Body:       `{"status": "ok"}`,		StatusCode: 200,	return events.APIGatewayProxyResponse{	log.Printf("Received %s event for %s", event.Action, event.Repository.FullName)	}		return events.APIGatewayProxyResponse{StatusCode: 400}, err	if err := json.Unmarshal([]byte(request.Body), &event); err != nil {	var event GitHubEventfunc handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {}