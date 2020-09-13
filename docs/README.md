# AWS IAM roles for GitHub Actions workflows

## Background and rationale

GitHub Actions are a pretty nice solution for CI/CD. Where they fall short is
integration with other services, like AWS. The approach suggested by AWS is to
create an IAM **user**, allocate it a long-lived access key and store those
credentials in GitHub's secret storage. This is undesirable for folks working
in an environment where IAM **users** are not permitted. 

This repo is a GitHub action that can grant your workflows access to AWS via
an AWS IAM role session. This means no need to store long-lived credentials
in GitHub and comes with a few other benefits.

## Usage

### Inside AWS

[`api.yml`](/api.yml) is a CloudFormation template to deploy in one of your
AWS accounts. It has three resources:

* `GithubSecret` is an `AWS::SecretsManager::Secret` for storing credentials
  to access the GitHub API. It needs both a GitHub [personal access token][pat]
  and the `user_session` cookie for github.com from a logged-in user.
  
* `ExampleRoleForGithub` is an `AWS::IAM::Role` to demonstrate how you can
  create a role that is assumable by a GitHub Actions workflow. Specifically it
  demonstrates the role having tags and allowing the Lambda function to pass
  role session tags. 
  
* `Function` is an `AWS::Serverless::Function` that creates both an API Gateway
  and a Lambda function as its backend. It has an example IAM policy and set
  of role session tags that are likely useful.
  
### In your GitHub Action workflows

```yaml
on:
  - push
name: example
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: keygen
        uses: glassechidna/actions2aws/keygen@v0.1

      - name: assume role
        uses: glassechidna/actions2aws/assume@v0.1
        with:
          step: keygen
          url: ${{ secrets.URL }}
          role: arn:aws:iam::${{ secrets.ACCOUNT_ID }}:role/actions2aws-ExampleRoleForGithub
          # these values should look something like:  
          # url: https://someexample.execute-api.ap-southeast-2.amazonaws.com/
          # role: arn:aws:iam::0123456789012:role/actions2aws-ExampleRoleForGithub

      - run: aws sts get-caller-identity # or whatever else you want
```

## Things worth knowing

* The github.com `user_session` cookie value is needed because the GitHub public
  API currently [won't return logs for an in-progress workflow run][api-issue].
  
* How you define your role's trust policies is really up to you. My recommendations
  would be (these are all defined in [`api.yml`](/api.yml)):
  * Restrict the Lambda function's role to only be allowed to assume specific
    roles. In my case, I don't have a naming scheme - so I limit it to roles with
    a `github:actions-role = true` tag.
  * Restrict the Lambda function's role to only be allowed to pass particular
    session tags (e.g. only `github:*`)
  * Match the above session tag restrictions on the target role side too, i.e.
    limit the allowable session tags in the role's trust policy.
  * Limit target roles to only being assumable by the Lambda function, rather
    than the entire account hosting the Lambda function.

## How it works

tl;dr:

* The `keygen` action generates an [`age`][age] public-private keypair, saves 
  the private half to disk and logs the public half to the console.
* The `request` action invokes the Lambda (via the API Gateway) with a POST
  body that identifies which repo, workflow and run it is.
* The Lambda uses this POST body to lookup information about the in-progress
  run (including its public key logged to the console) and assume a role for 
  the run.
* The Lambda returns the assumed role session credentials *encrypted with the
  public key displayed in the logs for the `keygen` step*. This ensures that
  no one else can receive valid credentials on behalf of that workflow run.
* The `request` action then sets `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
  and `AWS_SESSION_TOKEN` environment variables for future steps.

![sequence diagram](/docs/sequence.png)

[pat]: https://github.com/settings/tokens
[api-issue]: https://github.community/t/logs-and-artifacts-not-available-in-api-for-in-progress-runs/132091
[age]: https://age-encryption.org/
