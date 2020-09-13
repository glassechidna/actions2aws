package main

import (
	"bytes"
	"encoding/json"
	"filippo.io/age"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	"github.com/glassechidna/actions2aws"
	"github.com/glassechidna/lambdahttp/pkg/gowrap"
	"github.com/glassechidna/lambdahttp/pkg/secretenv"
	"github.com/jmespath/go-jmespath"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

var DefaultTagsExpression = `{
	"github:jobId":  to_string(job.id),
	"github:runId":  to_string(run.id),
	"github:run":    to_string(run.run_number),
	"github:job":    job.name,
	"github:commit": run.head_commit.id,
	"github:repo":   run.repository.full_name,
	"github:author": run.head_commit.author.email
}`

func main() {
	sess, err := session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
	if err != nil {
		panic(err)
	}

	secretenv.MutateEnv(sess)

	api := &Api{
		githubToken:  os.Getenv("GITHUB_API_TOKEN"),
		userSession:  os.Getenv("GITHUB_USER_SESSION"),
		permittedOrg: os.Getenv("PERMITTED_GITHUB_ORG"),
		stsApi:       sts.New(sess),
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		body := actions2aws.RequestPayload{}
		err = json.Unmarshal(bodyBytes, &body)
		if err != nil {
			panic(err)
		}

		encryptedResp, err := api.assumeRole(body)
		if err != nil {
			panic(err)
		}

		w.Write(encryptedResp)
	})

	lambda.StartHandler(gowrap.ApiGateway(http.DefaultServeMux))
}

type Api struct {
	githubToken  string
	userSession  string
	permittedOrg string
	stsApi       stsiface.STSAPI
}

func (api *Api) assumeRole(body actions2aws.RequestPayload) ([]byte, error) {
	runBytes, err := api.getRun(body.Repo, body.RunId)
	if err != nil {
		return nil, err
	}

	run := runResponse{}
	_ = json.Unmarshal(runBytes, &run)

	if run.Repository.ID != run.HeadRepository.ID && run.Repository.ID != 0 {
		// this is a fork, no credentials for you
		return nil, errors.New("no credentials for a fork")
	}

	repoParts := strings.SplitN(body.Repo, "/", 2)
	if repoParts[0] != api.permittedOrg {
		return nil, errors.New("no credentials for incorrect org")
	}

	jobBytes, err := api.getJobs(body.Repo, body.RunId)
	if err != nil {
		return nil, err
	}

	jobs := jobsResponse{}
	_ = json.Unmarshal(jobBytes, &jobs)

	jobIdx := -1
	for idx, job := range jobs.Jobs {
		if job.Name == body.JobName {
			jobIdx = idx
		}
	}
	if jobIdx == -1 {
		return nil, errors.New("job not found")
	}
	job := jobs.Jobs[jobIdx]

	stepNumber := -1
	for _, step := range job.Steps {
		if step.Name == body.StepName {
			stepNumber = step.Number
		}
	}
	if stepNumber == -1 {
		return nil, errors.New("step not found")
	}

	encryptionKey, err := api.getEncryptionKey(body.Repo, job.HeadSha, job.ID, stepNumber)
	if err != nil {
		return nil, err
	}

	tagMap, err := getTagMap(jobBytes, runBytes, jobIdx)
	if err != nil {
		return nil, err
	}

	roleSessionName := fmt.Sprintf("%s_%d", strings.ReplaceAll(body.Repo, "/", "_"), run.RunNumber)

	c, err := api.getRoleCredentials(body.Repo, body.RoleARN, roleSessionName, tagMap)
	if err != nil {
		return nil, err
	}

	respBody, _ := json.Marshal(actions2aws.ResponsePayload{
		AccessKeyId:     *c.AccessKeyId,
		SecretAccessKey: *c.SecretAccessKey,
		SessionToken:    *c.SessionToken,
		Expiry:          *c.Expiration,
	})

	j, _ := json.Marshal(map[string]interface{}{
		"msg":         "issued aws credentials",
		"request":     body,
		"accessKeyId": *c.AccessKeyId,
		"expiry":      c.Expiration.String(),
		"tags":        tagMap,
	})
	fmt.Println(string(j))

	encryptedResp, err := encryptResponse(respBody, encryptionKey)
	if err != nil {
		return nil, err
	}

	return encryptedResp, nil
}

func getTagMap(jobBytes, runBytes []byte, jobIdx int) (map[string]string, error) {
	var jobs interface{}
	_ = json.Unmarshal(jobBytes, &jobs)
	job, err := jmespath.Search(fmt.Sprintf("jobs[%d]", jobIdx), jobs)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var run interface{}
	_ = json.Unmarshal(runBytes, &run)

	data := map[string]interface{}{"run": run, "job": job}

	tagsExpression := DefaultTagsExpression
	if expr := os.Getenv("TAGS_JMESPATH"); len(expr) > 0 {
		tagsExpression = expr
	}

	msi, err := jmespath.Search(tagsExpression, data)

	m := map[string]string{}
	for key, val := range msi.(map[string]interface{}) {
		m[key] = val.(string)
	}

	return m, nil
}

func encryptResponse(respBody []byte, encryptionKey string) ([]byte, error) {
	recip, err := age.ParseX25519Recipient(encryptionKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	buf := &bytes.Buffer{}
	encw, err := age.Encrypt(buf, recip)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = encw.Write(respBody)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = encw.Close()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return buf.Bytes(), nil
}

func (a *Api) getRoleCredentials(repo, roleARN, sessionName string, tagMap map[string]string) (*sts.Credentials, error) {
	tags := []*sts.Tag{}
	for k, v := range tagMap {
		tags = append(tags, &sts.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	stsResp, err := a.stsApi.AssumeRole(&sts.AssumeRoleInput{
		ExternalId:      &repo,
		RoleArn:         &roleARN,
		RoleSessionName: &sessionName,
		Tags:            tags,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return stsResp.Credentials, nil
}
