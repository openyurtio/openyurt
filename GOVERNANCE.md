# Governance

The governance model adopted in OpenYurt is influenced by many CNCF projects.

## Principles

- **Open**: OpenYurt is open source community. See [Contributor License Agreement](PATH to the CLA).
- **Welcoming and respectful**: See [Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).
- **Transparent and accessible**: Work and collaboration should be done in public.
- **Merit**: Ideas and contributions are accepted according to their technical merit
  and alignment with project objectives, scope and design principles.

## Project Lead

The OpenYurt project has a project lead. The lead is a single person that has a final say in any decision concerning the OpenYurt project.

The term of the project lead is one year, with no term limit restriction.

The project lead is elected by OpenYurt maintainers according to an individual's technical merit to the OpenYurt project.

The current project lead is identified in the [MAINTAINERS](MAINTAINERS.md) file.


## Process for becoming a maintainer

* Express interest to the [project lead](MAINTAINERS.md) that you are interested in becoming a
  maintainer. Becoming a maintainer generally means that you are going to be spending substantial
  time on OpenYurt  for the foreseeable future. You are expected to have domain expertise and be extremely
  proficient in golang. 
* We will expect you to start contributing increasingly complicated PRs, under the guidance
  of the existing maintainers.
* We may ask you to do some PRs from our backlog. As you gain experience with the code base and our standards, 
  we will ask you to do code reviews for incoming PRs.
* After a period of approximately two months of working together and making sure we see eye to eye,
  the project lead will confer and decide whether to grant maintainer status or not.
  We make no guarantees on the length of time this will take, but two months is an approximate
  goal.


## Maintainer responsibilities

* Classify GitHub issues and perform pull request reviews for other maintainers and the community.

* During GitHub issue classification, apply all applicable [labels](https://github.com/alibaba/openyurt/labels)
  to each new issue. Labels are extremely useful for follow-up of future issues. Which labels to apply
  is somewhat subjective so just use your best judgment. 

* Make sure that ongoing PRs are moving forward at the right pace or closing them if they are not
  moving in a productive direction.

* Participate when called upon in the security release process. Note
  that although this should be a rare occurrence, if a serious vulnerability is found, the process
  may take up to several full days of work to implement.


## When does a maintainer lose maintainer status

* If a maintainer is no longer interested or cannot perform the maintainer duties listed above, they
should volunteer to be moved to emeritus status. 

* In extreme cases this can also occur by a vote of the maintainers per the voting process. The voting 
process is a simple majority in which each maintainer receives one vote.


## Changes in Project Lead

Changes in project lead is initiated by opening a github PR.

Anyone from OpenYurt community can vote on the PR with either +1 or -1.

Only the following votes are binding:
1) Any maintainer that has been listed in the [MAINTAINERS](MAINTAINERS.md) file before the PR is opened.
2) Any maintainer from an organization may cast the vote for that organization. However, no organization
should have more binding votes than 1/5 of the total number of maintainers defined in 1).

The PR should only be opened no earlier than 6 weeks before the end of the project lead's term.
The PR should be kept open for no less than 4 weeks. The PR can only be merged after the end of the
last project lead's term, with more +1 than -1 in the binding votes.

When there are conflicting PRs about changes in project lead, the PR with the most binding +1 votes is merged.

The project lead can volunteer to step down.


## Decision making process

Decisions are build on consensus between maintainers.
Proposals and ideas can either be submitted for agreement via a github issue or PR,
or by sending an email to `openyurt@googlegroups.com`.

In general, we prefer that technical issues and maintainer membership are amicably worked out between the persons involved.
If a dispute cannot be decided independently, get a third-party maintainer (e.g. a mutual contact with some background
on the issue, but not involved in the conflict) to intercede.
If a dispute still cannot be decided, the project lead has the final say to decide an issue.

Decision making process should be transparent to adhere to the principles of OpenYurt project.

All proposals, ideas, and decisions by maintainers or the project lead
should either be part of a github issue or PR, or be sent to `openyurt@googlegroups.com`.


## Code of Conduct

The OpenYurt [Code of Conduct](CODE_OF_CONDUCT.md) is aligned with the CNCF Code of Conduct.

## Credits

Sections of this documents have been borrowed from [Fluentd](https://github.com/fluent/fluentd/blob/master/GOVERNANCE.md) and [CoreDNS](https://github.com/coredns/coredns/blob/master/GOVERNANCE.md) projects.
