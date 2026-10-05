#!/bin/bash
# Merges pull request $PR, with a merge commit, if it is open, labeled
# merge-when-green, its head approved (a merge-when-green commit status, put
# on the head the label went on), green on its head, and holding its base's
# tip (see workflows/merge-when-green.yml). Two callers:
# - the test workflow, at the end of a green run, with TESTED set to the head
#   it just tested;
# - the label going on, without TESTED: then the test workflow's latest run on
#   the head must be green. One still running is waited for up to WAIT
#   seconds, for a label that goes on just after the run looked; a run with
#   longer to go looks at the label itself when it ends.
set -euo pipefail

info=$(gh pr view "$PR" --repo "$GITHUB_REPOSITORY" --json headRefOid,baseRefName,title,state,labels)
head=$(jq -r .headRefOid <<<"$info")
base=$(jq -r .baseRefName <<<"$info")
title=$(jq -r .title <<<"$info")
if [ "$(jq -r .state <<<"$info")" != OPEN ] || [ "$(jq '[.labels[].name] | index("merge-when-green") != null' <<<"$info")" != true ]; then
	echo "pull request $PR is not open, or not labeled merge-when-green"
	exit 0
fi
approved=$(gh api "repos/$GITHUB_REPOSITORY/commits/$head/statuses" --jq '[.[] | select(.context == "merge-when-green" and .state == "success")] | length')
if [ "$approved" = 0 ]; then
	echo "$head isn't approved: the label went on before it was pushed, or its job hasn't run yet"
	exit 0
fi

if [ -n "${TESTED:-}" ]; then
	if [ "$TESTED" != "$head" ]; then
		echo "pull request $PR has moved on from $TESTED, which this run tested"
		exit 0
	fi
else
	waited=0
	while :; do
		run=$(gh run list --repo "$GITHUB_REPOSITORY" --workflow test.yml --commit "$head" --event pull_request --limit 1 --json status,conclusion)
		if [ "$(jq -r '.[0].status' <<<"$run")" = completed ] || [ "$waited" -ge "${WAIT:-0}" ]; then
			break
		fi
		sleep 10
		waited=$((waited + 10))
	done
	if [ "$(jq -r '.[0].status' <<<"$run")" != completed ] || [ "$(jq -r '.[0].conclusion' <<<"$run")" != success ]; then
		echo "CI is not green on $head yet; its run merges it when it ends green"
		exit 0
	fi
fi

# The head must hold the base's tip: then merging it changes nothing CI
# didn't test.
tip=$(gh api "repos/$GITHUB_REPOSITORY/git/ref/heads/$base" --jq .object.sha)
status=$(gh api "repos/$GITHUB_REPOSITORY/compare/$tip...$head" --jq .status)
if [ "$status" != ahead ] && [ "$status" != identical ]; then
	gh pr comment "$PR" --repo "$GITHUB_REPOSITORY" --body "Not merged: $base has moved on ($tip) since this branch left it, so its merge with $base isn't what CI tested. Bring $base into the branch; that push takes the label off, for a fresh approval."
	exit 0
fi
gh pr merge "$PR" --repo "$GITHUB_REPOSITORY" --merge --match-head-commit "$head" --subject "Merge $title" --body ""
echo "merged pull request $PR at $head"
