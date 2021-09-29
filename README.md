This used to be kludgy workaround for assuming an AWS IAM role in GitHub Actions
without needing to use secrets, but it has since been made obsolete by GitHub's
native support for [federation](https://awsteele.com/blog/2021/09/15/aws-federation-comes-to-github-actions.html).

The original code remains in the repo, just look at the git history.
