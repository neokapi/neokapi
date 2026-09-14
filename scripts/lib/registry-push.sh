# shellcheck shell=bash
# Commit an update to a clone of neokapi/registry and push it, retrying when
# another release pushed to the registry first.
#
# Release workflows that run at the same time (a kapi tag and a bowrain tag cut
# together) each clone the registry, write their index and push. The push that
# lands second is rejected because main moved under it. On a rejected push,
# registry_commit_and_push rebases onto the remote and pushes again.
#
# When the rebase conflicts, the helper takes the remote tree and runs the
# caller's update on it, so registry-update writes the index and nobody resolves
# JSON by hand. After a clean rebase it runs the update again as well and folds
# any change into the commit, so what lands is registry-update's output on the
# tree it lands on. registry-update is an upsert, so running it again is safe.
#
# Usage: registry_commit_and_push <clone> <message> <update-function> <path>...
#
#   <clone>            a clone with its upstream branch and user.name/user.email set
#   <update-function>  a shell function that writes the update into <clone>
#   <path>...          the files the update writes, relative to <clone>
#
# REGISTRY_PUSH_ATTEMPTS (default 5) bounds the pushes. Before attempt n+1 the
# helper waits n * REGISTRY_PUSH_BACKOFF seconds (default 5) plus up to one
# more backoff at random, so releases that collided once do not retry in step.
registry_commit_and_push() {
  local clone="$1" message="$2" update="$3"
  shift 3
  local attempts="${REGISTRY_PUSH_ATTEMPTS:-5}" backoff="${REGISTRY_PUSH_BACKOFF:-5}"
  local attempt=1 ahead
  while :; do
    "$update" || return 1
    git -C "$clone" add -- "$@" || return 1
    ahead="$(git -C "$clone" rev-list --count '@{upstream}..HEAD')" || return 1
    if ! git -C "$clone" diff --cached --quiet; then
      if [ "$ahead" -gt 0 ]; then
        git -C "$clone" commit -q --amend --no-edit || return 1
      else
        git -C "$clone" commit -q -m "$message" || return 1
      fi
      ahead=1
    fi
    if [ "$ahead" -eq 0 ]; then
      echo "registry: nothing to push for \"${message}\"; the registry already holds it"
      return 0
    fi
    if git -C "$clone" push -q; then
      echo "registry: pushed \"${message}\" on attempt ${attempt}"
      return 0
    fi
    if [ "$attempt" -ge "$attempts" ]; then
      echo "registry: the push of \"${message}\" was rejected ${attempts} times; giving up" >&2
      return 1
    fi
    echo "registry: push rejected on attempt ${attempt} of ${attempts}; rebasing onto the remote"
    sleep $((attempt * backoff + RANDOM % (backoff + 1)))
    attempt=$((attempt + 1))
    if ! git -C "$clone" pull -q --rebase; then
      echo "registry: the rebase conflicted; running the update again on the remote tree"
      git -C "$clone" rebase --abort 2>/dev/null || true
      git -C "$clone" reset -q --hard '@{upstream}' || return 1
    fi
  done
}
