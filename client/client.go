package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"text/template"

	"filippo.io/age"
	"github.com/glassechidna/actions2aws"
	"github.com/pkg/errors"
)

func main() {
	//exitEarlyOnForks()

	switch os.Args[1] {
	case "keygen":
		keygen()
	case "request":
		request()
	default:
		panic("unexpected subcmd")
	}
}

func exitEarlyOnForks() {
	body, err := ioutil.ReadFile(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		panic(err)
	}

	event := struct {
		PR struct {
			Head struct {
				Repo struct {
					ID int `json:"id"`
				} `json:"repo"`
			} `json:"head"`
			Base struct {
				Repo struct {
					ID int `json:"id"`
				} `json:"repo"`
			} `json:"base"`
		} `json:"pull_request"`
	}{}

	err = json.Unmarshal(body, &event)
	if err != nil {
		panic(err)
	}
}

func request() {
	payload := actions2aws.RequestPayload{
		Repo:     os.Getenv("GITHUB_REPOSITORY"),
		RunId:    os.Getenv("GITHUB_RUN_ID"),
		JobName:  os.Getenv("GITHUB_JOB"),
		RoleARN:  os.Getenv("ACTIONS2AWS_ROLE"),
		StepName: os.Getenv("ACTIONS2AWS_STEP_NAME"),
	}
	j, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", os.Getenv("ACTIONS2AWS_URL"), bytes.NewReader(j))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}

	if resp.StatusCode != 200 {
		panic(errors.New("unexpected status code"))
	}

	identity := decryptor()

	r, err := age.Decrypt(resp.Body, identity)
	if err != nil {
		panic(err)
	}

	decrypted, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}

	apiResp := actions2aws.ResponsePayload{}
	err = json.Unmarshal(decrypted, &apiResp)
	if err != nil {
		panic(err)
	}

	fmt.Printf(`
::add-mask::%s
::add-mask::%s
::add-mask::%s
`, apiResp.AccessKeyId, apiResp.SecretAccessKey, apiResp.SessionToken)

	tmpl, err := template.New("").Parse(`
AWS_ACCESS_KEY_ID={{.AccessKeyId}}
AWS_SECRET_ACCESS_KEY={{.SecretAccessKey}}
AWS_SESSION_TOKEN={{.SessionToken}}
`)
	if err != nil {
		panic(err)
	}

	f, err := os.OpenFile(os.Getenv("GITHUB_ENV"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}

	err = tmpl.Execute(f, apiResp)
	if err != nil {
		panic(err)
	}
}

func decryptor() *age.X25519Identity {
	key, err := ioutil.ReadFile(privateKeyPath())
	if err != nil {
		panic(err)
	}

	identity, err := age.ParseX25519Identity(string(key))
	if err != nil {
		panic(err)
	}
	return identity
}

func privateKeyPath() string {
	path := filepath.Join(os.Getenv("HOME"), ".actions2aws", "key")
	err := os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		panic(err)
	}

	return path
}

func keygen() {
	k, err := age.GenerateX25519Identity()
	if err != nil {
		panic(err)
	}

	fmt.Printf("ACTIONS2AWS PUBKEY: %s\n", k.Recipient())

	err = ioutil.WriteFile(privateKeyPath(), []byte(k.String()), 0600)
	if err != nil {
		panic(err)
	}
}
