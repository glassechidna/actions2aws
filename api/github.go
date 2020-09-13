package main

import (
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"
)

func (api *Api) getEncryptionKey(repo, commitSha string, jobId, stepIdx int) (string, error) {
	count := 0
attempt:
	jobLogsUrl := fmt.Sprintf("https://github.com/%s/commit/%s/checks/%d/logs/%d", repo, commitSha, jobId, stepIdx)

	req, err := http.NewRequest("GET", jobLogsUrl, nil)
	if err != nil {
		return "", errors.WithStack(err)
	}

	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(&http.Cookie{Name: "user_session", Value: api.userSession})

	jobLogsResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.WithStack(err)
	}

	if jobLogsResp.StatusCode == 404 && count < 5 {
		time.Sleep(time.Duration(count) * time.Second)
		count++
		goto attempt
	}

	jobLogBytes, err := ioutil.ReadAll(jobLogsResp.Body)
	if err != nil {
		return "", errors.WithStack(err)
	}

	regex := regexp.MustCompile(`ACTIONS2AWS PUBKEY: (\S+)`)
	matches := regex.FindStringSubmatch(string(jobLogBytes))
	pubkey := matches[1]

	return pubkey, nil
}

func (api *Api) getJobs(repo, runId string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/actions/runs/%s/jobs", repo, runId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "token "+api.githubToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	return body, errors.WithStack(err)
}

func (api *Api) getRun(repo, runId string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/actions/runs/%s", repo, runId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "token "+api.githubToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	return body, errors.WithStack(err)
}

type runResponse struct {
	ID            int           `json:"id"`
	NodeID        string        `json:"node_id"`
	HeadBranch    string        `json:"head_branch"`
	HeadSha       string        `json:"head_sha"`
	RunNumber     int           `json:"run_number"`
	Event         string        `json:"event"`
	Status        string        `json:"status"`
	Conclusion    string        `json:"conclusion"`
	WorkflowID    int           `json:"workflow_id"`
	URL           string        `json:"url"`
	HTMLURL       string        `json:"html_url"`
	PullRequests  []interface{} `json:"pull_requests"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	JobsURL       string        `json:"jobs_url"`
	LogsURL       string        `json:"logs_url"`
	CheckSuiteURL string        `json:"check_suite_url"`
	ArtifactsURL  string        `json:"artifacts_url"`
	CancelURL     string        `json:"cancel_url"`
	RerunURL      string        `json:"rerun_url"`
	WorkflowURL   string        `json:"workflow_url"`
	Repository    struct {
		ID int `json:"id"`
	} `json:"repository"`
	HeadRepository struct {
		ID int `json:"id"`
	} `json:"head_repository"`
}

type jobsResponse struct {
	Jobs []Job `json:"jobs"`
}

type Job struct {
	ID          int       `json:"id"`
	RunID       int       `json:"run_id"`
	RunURL      string    `json:"run_url"`
	NodeID      string    `json:"node_id"`
	HeadSha     string    `json:"head_sha"`
	URL         string    `json:"url"`
	HTMLURL     string    `json:"html_url"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Name        string    `json:"name"`
	Steps       []struct {
		Name        string    `json:"name"`
		Status      string    `json:"status"`
		Conclusion  string    `json:"conclusion"`
		Number      int       `json:"number"`
		StartedAt   time.Time `json:"started_at"`
		CompletedAt time.Time `json:"completed_at"`
	} `json:"steps"`
	CheckRunURL string `json:"check_run_url"`
}
