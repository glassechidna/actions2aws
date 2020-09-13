package actions2aws

import "time"

type ResponsePayload struct {
	AccessKeyId     string
	SecretAccessKey string
	SessionToken    string
	Expiry          time.Time
}

type OldRequestPayload struct {
	RunId        string
	RunNumber    string
	SHA          string
	Ref          string
	RepoName     string
	RepoOwner    string
	Action       string
	Workflow     string
	Actor        string
	Token        string
	RoleARN      string
	SerialNumber string
}

type RequestPayload struct {
	Repo     string
	RunId    string
	JobName  string
	StepName string
	RoleARN  string
}
