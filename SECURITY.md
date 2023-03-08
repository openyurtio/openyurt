# Security Policy

## Supported Versions

OpenYurt commits to supporting the n-2 version minor version of the current major release;
as well as the last minor version of the previous major release.

Here's an overview:

| Version | Supported           |
| ------- | ------------------- |
| 1.0.x   | :white_check_mark: |
| 0.7.x   | :white_check_mark:  |
| < 0.7   | :x:                 |

## Prevention

Container images are scanned in every pull request (PR) with [Trivy](https://github.com/aquasecurity/trivy) to detect new vulnerabilities.

OpenYurt maintainers are working to improve our prevention by adding additional measures:

- Scan code in master/nightly build and PR/master/nightly for Go.
- Scan published container images on Dockerhub Container Registry.

## Disclosures

We strive to ship secure software, but we need the community to help us find security breaches.

In case of a confirmed breach, reporters will get full credit and can be keep in the loop, if preferred.

### Private Disclosure Processes

We ask that all suspected vulnerabilities be privately and responsibly disclosed by [contacting our maintainers](mailto:security@mail.openyurt.io).

### Public Disclosure Processes

If you know of a publicly disclosed security vulnerability please IMMEDIATELY email the [OpenYurt maintainers](mailto:security@mail.openyurt.io) to inform about the vulnerability so they may start the patch, release, and communication process.

### Compensation

We do not provide compensations for reporting vulnerabilities except for eternal gratitude.

## Communication

[GitHub Security Advisory](https://github.com/openyurtio/openyurt/security/advisories) will be used to communicate during the process of identifying, fixing & shipping the mitigation of the vulnerability.

The advisory will only be made public when the patched version is released to inform the community of the breach and its potential security impact.