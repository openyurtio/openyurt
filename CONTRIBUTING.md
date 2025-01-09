# Contributing to OpenYurt

Welcome to join OpenYurt project. Here is the contributing guide for you.

## Code of Conduct

Please do check our [Code of Conduct](CODE_OF_CONDUCT.md) before making contributions.

## Topics

* [Reporting security issues](#reporting-security-issues)
* [Reporting general issues](#reporting-general-issues)
* [Code and doc contribution](#code-and-doc-contribution)
* [Review](#review)
* [Engage to help anything](#engage-to-help-anything)
* [At Last](#at-last)

## Reporting security issues

We take security issues seriously and discourage anyone to spread security issues. If you find a security issue in OpenYurt, please do not discuss it in public and even do not open a public issue. Instead we encourage you to send us a private email to [security@mail.openyurt.io](mailto:security@mail.openyurt.io) to report the security issue.

## Reporting general issues

Any OpenYurt user can potentially be a contributor. If you have any feedback for the project, feel free to open an issue. Steps are as follows:

1. Click `New issue` at [the issue interface](https://github.com/openyurtio/openyurt/issues) to create a new issue.
2. Select the kind of the issue and `Get started`.
3. Fill the title of the issue and the content of the given issue template.
4. Finally `Submit new issue`.

Since OpenYurt development will be collaborated in a distributed manner, we appreciate **WELL-WRITTEN**, **DETAILED**, **EXPLICIT** issue reports. To make communication more efficient, we suggest everyone to search if your issue is an existing one before filing a new issue. If you find it to be existing, please append your details in the issue comments.

There are lot of cases for which you could open an issue:

* Bug report
* Feature request
* Performance issues
* Feature proposal
* Feature design
* Help wanted
* Doc incomplete
* Test improvement
* Any questions about the project, and so on

Please remind that when filing a new issue, do remove the sensitive data from your post. Sensitive data could be password, secret key, network locations, private business data and so on.

## Code and doc contribution

Any action that may make OpenYurt better is encouraged. The action can be realized via a PR (short for pull request).

* If you find a typo, try to fix it!
* If you find a bug, try to fix it!
* If you find some redundant codes, try to remove them!
* If you find some test cases missing, try to add them!
* If you could enhance a feature, please **DO NOT** hesitate!
* If you find code implicit, try to add comments to make it clear!
* If you find tech debts, try to refactor them!
* If you find document incorrect, please fix that!

It is impossible to list them completely, we are looking forward to your pull requests.
Before coding, in order to avoid duplication of work, you had better to search the community to check if someone has been working on a same problem. 
Before submitting a PR, we suggest you could take a look at the PR rules here.

* [Workspace Preparation](#workspace-preparation)
* [Branch Definition](#branch-definition)
* [Commit Rules](#commit-rules)
* [PR Guidelines](#pr-guidelines)

### Workspace Preparation

We assume you have a GitHub ID already, then you could finish the preparation in the following steps:

1. **FORK** OpenYurt to your repository. To make this work, you just need to click the button `Fork` in top-right corner of [openyurt](https://github.com/openyurtio/openyurt) main page. Then you will end up with your repository in `https://github.com/<username>/openyurt`, in which `username` is your GitHub ID.
1. **CLONE** your own repository to develop locally. Use `git clone https://github.com/<username>/openyurt.git` to clone repository to your local machine. Then you can create new branches to finish the change you wish to make.
1. **Set Remote** upstream to be openyurt using the following two commands:

```
git remote add upstream https://github.com/openyurtio/openyurt.git
git remote set-url --push upstream no-pushing
```

With this remote setting, you can check your git remote configuration like this:

```
$ git remote -v
origin     https://github.com/<username>/openyurt.git (fetch)
origin     https://github.com/<username>/openyurt.git (push)
upstream   https://github.com/openyurtio/openyurt.git (fetch)
upstream   no-pushing (push)
```

With above, we can easily synchronize local branches with upstream branches.

### Branch Definition

Right now we assume every contribution via pull request is for the `master` branch in OpenYurt.
There are several other branches such as rc branches, release branches and backport branches.
Before officially releasing a version, we may checkout a rc (release candidate) branch for more testings.
When officially releasing a version, there may be a release branch before tagging which will be deleted after tagging.
When backporting some fixes to existing released version, we will checkout backport branches.

### Commit Rules

In OpenYurt, we take two rules seriously for submitted PRs:

* [Commit Message](#commit-message)
* [Commit Content](#commit-content)

#### Commit Message

Commit message could help reviewers better understand what the purpose of submitted PR is. It could help accelerate the code review procedure as well. We encourage contributors to use **EXPLICIT** commit message rather than ambiguous message. In general, we advocate the following commit message type:

* Docs: xxxx. For example, "Docs: add docs about storage installation".
* Feature: xxxx.For example, "Feature: make result show in sorted order".
* Bugfix: xxxx. For example, "Bugfix: fix panic when input nil parameter".
* Style: xxxx. For example, "Style: format the code style of Constants.java".
* Refactor: xxxx. For example, "Refactor: simplify to make codes more readable".
* Test: xxx. For example, "Test: add unit test case for func InsertIntoArray".
* Other readable and explicit expression ways.

On the other hand, we discourage contributors to write committing messages using the following ways:

* ~~fix bug~~
* ~~update~~
* ~~add doc~~

#### Commit Content

Commit content represents all content changes included in one commit. We had better include things in one single commit which could support reviewer's complete review without any other commits' help. In another word, contents in one single commit can pass the CI to avoid code mess. In brief, there are two minor rules for us to keep in mind:

* Avoid very large change in a commit;
* Be complete and reviewable for each commit.

### PR Guidelines

PR is the only way to make change to OpenYurt project. Before submitting a Pull Request, you should check your local git repository and keep pace with the OpenYurt repo to avoid the merge conflict. In addition, you should have some knowledge of how does the OpenYurt [CI Workflow](https://openyurt.io/docs/developer-manuals/ci-workflow) work.

After committing to your forked OpenYurt repository, you can submit a pull request to the official OpenYurt repository, asking for the merge of your change. Steps are as follows:

1. Push the code at your local host to your forked OpenYurt repository.
2. Login the Github and enter your OpenYurt repository.
3. Click `New pull request` at the pull request interface and select your branch to merge.
4. Click `Create pull request` and fill the content of the given pull request template.
5. Finally click `Create pull request` to submit the pull request, and you can find it at [the pull request interface of OpenYurt](https://github.com/openyurtio/openyurt/pulls).

In most cases, one pull request should only focus on one work, such as fixing a bug. Thus, only one commit should be contained in one pull request. You should amend your pull request if you find that there are more than one commits in it, using `git reset` and `git commit` at your local host. After your amending, you can push it to your forked openyurt repository through `git push`(usually need to do forcely, take caution). The submitted pull request will sync with the branch you select to merge(at step 3), and no need to create a new pull request.

You should check the CI workflow after submitting your pull request and make all the check passed. Then, you just need to wait for the review and approval from community members. If the community accepts your pull request, it will be labeled as `lgtm`(looks good to me) and `approve`.

## Review

Review means check others' pull requests. Everyone are welcome to take part in the review work. It's simple than pull request. You can leave your comment at the code you have interest in. Steps are as follows:

1. Select a pull request at [the pull request interface](https://github.com/openyurtio/openyurt/pulls).
2. Click `Files changed` to check what change the pull request introduced.
3. Click `+` at the left of the code line and leave your comment. At the first time, you need to click `Start a review`, and later you can click `Add review comment`.
4. Click `Finish your review` on the top right and `Submit review`.

Then you can just wait for the reply from the author of this pull request.

## Engage to help anything

GitHub is the primary place for OpenYurt contributors to collaborate. Although contributions via PR is an explicit way to help, we still call for any other types of helps.

* Reply to other's issues if you could;
* Help solve other user's problems;
* Help review other's PR design;
* Help review other's codes in PR;
* Discuss about OpenYurt to make things clearer;
* Advocate OpenYurt technology beyond GitHub;
* Write blogs on OpenYurt, and so on.

In a word, **ANY HELP CAN BE A CONTRIBUTION.**

## At Last

The openyurt is the only one of the repositories under [the openyurtio organization](https://github.com/openyurtio), all of these repositories consist the OpenYurt. Welcome to explore capabilities of each repository and make OpenYurt better.