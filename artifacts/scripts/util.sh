#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This file includes functions shared by the each repository's publish scripts.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# sync_repo() cherry picks the latest changes in k8s.io/kubernetes/<repo> to the
# local copy of the repository to be published.
#
# Prerequisites
# =============
#
# 1. we are in the root of the repository to be published
# 2. we are on the branch to be published (let's call it "destination branch"), possibly this
#    is an orphaned branch for new branches.
#
# Overall Flow
# ============
#
# 1. Fetch the current level of k8s.io/kubernetes.
# 2. Check out the ${src_branch} of k8s.io/kubernetes as branch filter-branch.
# 3. Rewrite the history of branch filter-branch to *only* include code in ${subdirectory},
#    keeping the original, corresponding k8s.io/kubernetes commit hashes
#    via "Kubernetes-commit: <kube-sha>" lines in the commit messages.
# 4. Locate all commits between the last time we sync'ed and now on the mainline, i.e. using
#    first-parent ancestors only, not going into a PR branch.
# 5. Switch back to the ${dst_branch}
# 6. For each commit C on the mainline (= ${f_mainline_commit} in the code) identified in 4:
#    a) If C is a merge commit:
#       i)   Find the latest branching point B from the mainline leading to C as its second parent.
#       ii)  If there is another merge between B and C via the path of (i), apply the diff between
#            the latest branching point B and the last merge M on that path.
#       iii) Continue cherry-picking all single commits between B (if (ii) didn’t apply) or M
#            (by (ii) these are no merges).
#       iv)  Commit the merge commit C as empty fast-forward merge of the latest branching point
#            and the HEAD produced from of (ii) and (iii).
#    b) If C is not a merge commit:
#       i)   Find the corresponding k8s.io/kubernetes commit k_C and find its merge point k_M
#            with the mainline in k8s.io/kubernetes.
#       ii)  Cherry-pick k_C.
#       iii) Continue with the next C in 6, let’s call it C’. If
#            - there is no such C’  or
#            - C’ is a merge commit or
#            - the mainline merge point k_M’ of the corresponding k8s.io/kubernetes commit k_C’
#              is different from k_M,
#            cherry-pick k_M as fast-forward merge commit of the mainline and C.
#
# Dropped PR Merges
# =================
#
# The logic of 6b is necessary because git filter-branch drops fast-forward merges, leading to
# a picture like in the next section..
#
# With 6b we get a perfectly uniform sequence of branches and fast-forward merges, i.e. without
# any interleaving. For this to work it is essential that Github’s merge commit are clean as
# described above.
#
# Result
# ======
#
# This gives the following picture. All merges on the mainline are fast-forward merges.
# All branches of the second parents are linear.
#
#   M─┐ [master] Merge pull request #51467 from liggitt/client-go-owner
#   │ o Add liggitt to client-go approvers
#   │─┘
#   M─┐ Merge pull request #50562 from atlassian/call-cleanup-properly
#   │ o Call the right cleanup function
#   │─┘
#   M─┐ Merge pull request #51154 from RenaudWasTaken/gRPC-updated-1-3-0
#   │ o Bumped gRPC version to 1.3.0
#   │ o sync: reset go.mod
#   │─┘
#
# Master Merges
# =============
#
# The is a special case that a merge on the mainline of a non-master branch is a merge with
# the master branch. In that case, this master-merge is recreated on the ${dst_branch}
# pointing to the dst master branch with the second parent:
#
#   M─┐ {release-5.0} Merge remote-tracking branch 'origin/master' into release-1.8
#   │ M─┐ {master} Merge pull request #51876 from smarterclayton/disable_client_paging
#   │ │ o Disable default paging in list watches
#   o │ │ Kubernetes version v1.8.0-beta.1 file updates
#   M─┤ │ Merge remote-tracking branch 'origin/master' into release-1.8
#   │ │─┘
#   │ M─┐ Merge pull request #50708 from DirectXMan12/versions/autoscaling-v2beta1
#   │ │ o Move Autoscaling v2{alpha1 --> beta1}
#   │ │─┘
#   │ I─┐ Merge pull request #51795 from dims/bug-fix-51755
#   │ │ o Bug Fix - Adding an allowed address pair wipes port security groups
#   │ │ o sync: reset go.mod
#   │ │─┘
#
# Code Conventions
# ================
#
# 1. variables prefixed with
#    - k_ refer to k8s.io/kubernetes
#    - f_ refer to the filtered-branch, i.e. rewritten using filtered-branch
#    - dst_ refer to the published repository.
# 2. there are the functions
#    - kube-commit to map a f_..._commit or dst_..._commit to the corresponding
#      k_..._commmit using the "Kubernetes-commit: <sha>" line in the commit message,
#    - branch-commit to map a k_..._commit to a f_..._commit or dst_..._commit
#      (depending on the current branch or the second parameter if given.
sync_repo() {
    # subdirectory in k8s.io/kubernetes, e.g., staging/src/k8s.io/apimachinery
    local source_repo_org="${1}"
    local source_repo_name="${2}"
    local subdirectory="${3}"
    local src_branch="${4}"
    local dst_branch="${5}"
    local deps="${6:-""}"
    local required_packages="${7:-""}"
    local base_package="${8:-"k8s.io"}"
    local is_library="${9}"

    shift 9

    local recursive_delete_pattern="${1}"

    local commit_msg_tag="${source_repo_name^}-commit"
    readonly subdirectory src_branch dst_branch deps is_library

    local new_branch="false"
    local orphan="false"
    if ! git rev-parse -q --verify HEAD; then
        echo "Found repo without ${dst_branch} branch, creating initial commit."
        git commit -m "Initial commit" --allow-empty
        new_branch="true"
        orphan="true"
    elif [ $(ls -1 | wc -l) = 0 ]; then
        echo "Found repo without files, assuming it's new."
        new_branch="true"
    else
        echo "Starting at existing ${dst_branch} commit $(git rev-parse HEAD)."
    fi

    # checkout $src_branch, name it filtered-branch
    git branch -D filtered-branch >/dev/null || true
    git branch -f upstream-branch upstream/"${src_branch}"
    echo "Checked out source commit $(git rev-parse upstream-branch)."
    git checkout -q upstream-branch -b filtered-branch
    git reset -q --hard upstream-branch

    # filter filtered-branch (= ${src_branch}) by ${subdirectory} modifying commits
    # and rewrite paths. Each filtered commit (which is not dropped), gets the
    # original k8s.io/kubernetes commit hash in the commit message as "Kubernetes-commit: <hash>".
    # Then select all new mainline commits on filtered-branch as ${f_mainline_commits}
    # to loop through them later.
    local f_mainline_commits=""
    if [ "${new_branch}" = "true" ] && [ "${src_branch}" = master ]; then
        # new master branch
        filter-branch "${commit_msg_tag}" "${subdirectory}" "${recursive_delete_pattern}" ${src_branch} filtered-branch

        # find commits on the main line (will mostly be merges, but could be non-merges if filter-branch dropped
        # the corresponding fast-forward merge and left the feature branch commits)
        f_mainline_commits=$(git log --first-parent --format='%H' --reverse HEAD)

        # create and checkout new, empty master branch. We only need this non-orphan case for the master
        # as that usually exists for new repos.
        if [ ${orphan} = true ]; then
            git checkout -q --orphan ${dst_branch}
        else
            git checkout -q ${dst_branch}
        fi
    else
        # create filtered-branch-base before filtering for
        # - new branches that branch off master (i.e. the branching point)
        # - old branch which continue with the last old commit.
        if [ "${new_branch}" = "true" ]; then
            # new non-master branch
            local k_branch_point_commit=$(git-fork-point upstream/${src_branch} upstream/master)
            if [ -z "${k_branch_point_commit}" ]; then
                echo "Couldn't find a branch point of upstream/${src_branch} and upstream/master."
                return 1
            fi

            # does ${subdirectory} exist at ${k_branch_point_commit}? If not it was introduced to the branch via some fast-forward merge.
            # we use the fast-forward merge commit's second parent (on master) as branch point.
            if [ $(git ls-tree --name-only -r ${k_branch_point_commit} -- "${subdirectory}" | wc -l) = 0 ]; then
                echo "Subdirectory ${subdirectory} did not exist at branch point ${k_branch_point_commit}. Looking for fast-forward merge introducing it."
                last_with_subdir=$(git rev-list upstream/${src_branch} --first-parent --remove-empty -- "${subdirectory}" | tail -1)
                if [ -z "${last_with_subdir}" ]; then
                    echo "Couldn't find any commit introducing ${subdirectory} on branch upstream/${src_branch}"
                    return 1
                fi
                if ! is-merge ${last_with_subdir}; then
                    echo "Subdirectory ${subdirectory} was introduced on non-merge branch commit ${last_with_subdir}. We don't support this."
                    return 1
                fi
                k_branch_point_commit=$(git rev-parse ${last_with_subdir}^2)
                echo "Using second-parent ${k_branch_point_commit} of merge ${last_with_subdir} on upstream/${src_branch} as starting point for new branch"
            fi

            echo "Using branch point ${k_branch_point_commit} as new starting point for new branch ${dst_branch}."
            git branch -f filtered-branch-base ${k_branch_point_commit} >/dev/null

            echo "Rewriting upstream branch ${src_branch} to only include commits for ${subdirectory}."
            filter-branch "${commit_msg_tag}" "${subdirectory}" "${recursive_delete_pattern}" filtered-branch filtered-branch-base

            # for a new branch that is not master: map filtered-branch-base to our ${dst_branch} as ${dst_branch_point_commit}
            local k_branch_point_commit=$(kube-commit ${commit_msg_tag} filtered-branch-base) # k_branch_point_commit will probably be different than the k_branch_point_commit
                                                                                              # above because filtered drops commits and maps to ancestors if necessary

            local pick_dst_branch_point_commit_args=""
            if pick-single-dst-branch-point-commit ${k_branch_point_commit}; then
                pick_dst_branch_point_commit_args="-n1"
                echo "Considering only single destination branch point commit at ${k_branch_point_commit} for ${dst_branch}."
            fi

            local dst_branch_point_commit=$(branch-commit ${commit_msg_tag} ${k_branch_point_commit} master ${pick_dst_branch_point_commit_args})
            if [ -z "${dst_branch_point_commit}" ]; then
                echo "Couldn't find a corresponding branch point commit for ${k_branch_point_commit} as ascendent of origin/master."
                return 1
            fi

            git branch -f ${dst_branch} ${dst_branch_point_commit} >/dev/null
        else
            # old branch
            local k_base_commit="$(last-kube-commit ${commit_msg_tag} ${dst_branch} || true)"
            if [ -z "${k_base_commit}" ]; then
                echo "Couldn't find a ${commit_msg_tag} commit SHA in any commit on ${dst_branch}."
                return 1
            fi
            local k_base_merge=$(git-find-merge ${k_base_commit} upstream/${src_branch})
            if [ -z "${k_base_merge}" ]; then
                echo "Didn't find merge commit of source commit ${k_base_commit}. Odd."
                return 1
            fi
            git branch -f filtered-branch-base ${k_base_merge} >/dev/null

            echo "Rewriting upstream branch ${src_branch} to only include commits for ${subdirectory}."
            filter-branch "${commit_msg_tag}" "${subdirectory}" "${recursive_delete_pattern}" filtered-branch filtered-branch-base
        fi

        # find commits on the main line (will mostly be merges, but could be non-merges if filter-branch dropped
        # the corresponding fast-forward merge and left the feature branch commits)
        local f_base_commit=$(git rev-parse filtered-branch-base)
        f_mainline_commits=$(git log --first-parent --format='%H' --reverse ${f_base_commit}..HEAD)

        # checkout our dst branch. For old branches this is the old HEAD, for new non-master branches this is branch point on master.
        echo "Checking out branch ${dst_branch}."
        git checkout -q ${dst_branch}
    fi

    # remove old kubernetes-sha
    # TODO: remove once we are sure that no branches with kubernetes-sha exist anymore
    if [ -f kubernetes-sha ]; then
        git rm -q kubernetes-sha
        git commit -q -m "sync: remove kubernetes-sha"
    fi

    # remove existing recursive-delete-pattern files. After a first removal commit, the filter-branch command
    # will filter them out from upstream commits.
    apply-recursive-delete-pattern "${recursive_delete_pattern}"

    local dst_old_head=$(git rev-parse HEAD) # will be the initial commit for new branch

    # apply all PRs
    local k_pending_merge_commit=""
    local dst_needs_gomod_update=${new_branch} # has there been a go.mod reset which requires a complete go.mod update?
    local dst_merge_point_commit=$(git rev-parse HEAD) # the ${dst_branch} HEAD after the last applied f_mainline_commit
    for f_mainline_commit in ${f_mainline_commits} FLUSH_PENDING_MERGE_COMMIT; do
        local k_mainline_commit=""
        local k_new_pending_merge_commit=""

        if [ ${f_mainline_commit} = FLUSH_PENDING_MERGE_COMMIT ]; then
            # enforce that the pending merge commit is flushed
            k_new_pending_merge_commit=FLUSH_PENDING_MERGE_COMMIT
        else
            k_mainline_commit=$(kube-commit ${commit_msg_tag} ${f_mainline_commit})

            # check under which merge with the mainline ${k_mainline_commit}) is
            k_new_pending_merge_commit=$(git-find-merge ${k_mainline_commit} upstream-branch)
            if [ "${k_new_pending_merge_commit}" = "${k_mainline_commit}" ]; then
                # it's on the mainline itself, no merge above it
                k_new_pending_merge_commit=""
            fi
            if [ ${dst_branch} != master ] && is-merge-with-master "${k_mainline_commit}"; then
                # merges with master on non-master branches we always handle as pending merge commit.
                k_new_pending_merge_commit=${k_mainline_commit}
            fi
        fi
        if [ -n "${k_pending_merge_commit}" ] && [ "${k_new_pending_merge_commit}" != "${k_pending_merge_commit}" ]; then
            # the new pending merge commit is different than the old one. Apply the old one. Three cases:
            # a) it's a merge with master on a non-master branch
            #    (i) it's on the filtered-branch
            #    (ii) it's dropped on the filtered-branch, i.e. fast-forward
            # b) it's another merge
            local dst_parent2="HEAD"
            if [ ${dst_branch} != master ] && is-merge-with-master "${k_pending_merge_commit}"; then
                # it's a merge with master. Recreate this merge on ${dst_branch} with ${dst_parent2} as second parent on the master branch
                local k_parent2="$(git rev-parse ${k_pending_merge_commit}^2)"
                read k_parent2 dst_parent2 <<<$(look -b ${k_parent2} ../kube-commits-$(basename "${PWD}")-master)
                if [ -z "${dst_parent2}" ]; then
                    echo "Corresponding $(dirname ${PWD}) master branch commit not found for upstream master merge ${k_pending_merge_commit}. Odd."
                    return 1
                fi

                f_pending_merge_commit=$(branch-commit ${commit_msg_tag} ${k_pending_merge_commit} filtered-branch)
                if [ -n "${f_pending_merge_commit}" ]; then
                    echo "Cherry-picking source master-merge  ${k_pending_merge_commit}: $(commit-subject ${k_pending_merge_commit})."

                    # cherry-pick the difference on the filtered mainline
                    reset-gomod ${f_pending_merge_commit}^1 # unconditionally reset go.mod
                    dst_needs_gomod_update=true
                    if ! GIT_COMMITTER_DATE="$(commit-date ${f_pending_merge_commit})" git cherry-pick --keep-redundant-commits -m 1 ${f_pending_merge_commit} >/dev/null; then
                        echo
                        show-working-dir-status
                        return 1
                    fi
                    squash 2
                else
                    # the merge commit with master was dropped. This means it was a fast-forward merge,
                    # which means we can just re-use the tree on the dst master branch.
                    echo "Cherry-picking source dropped-master-merge ${k_pending_merge_commit}: $(commit-subject ${k_pending_merge_commit})."
                    git reset -q --hard ${dst_parent2}
                fi
            else
                echo "Cherry-picking source dropped-merge ${k_pending_merge_commit}: $(commit-subject ${k_pending_merge_commit})."
            fi
            local date=$(commit-date ${k_pending_merge_commit}) # author and committer date is equal for PR merges
            local dst_new_merge=$(GIT_COMMITTER_DATE="${date}" GIT_AUTHOR_DATE="${date}" git commit-tree -p ${dst_merge_point_commit} -p ${dst_parent2} -m "$(commit-message ${k_pending_merge_commit}; echo; echo "${commit_msg_tag}: ${k_pending_merge_commit}")" HEAD^{tree})
            # no amend-gomod needed here: because the merge-commit was dropped, both parents had the same tree, i.e. go.mod did not change.
            git reset -q --hard ${dst_new_merge}
            if ! skip-gomod-update ${k_pending_merge_commit}; then
                fix-gomod "${deps}" "${required_packages}" "${base_package}" "${is_library}" ${dst_needs_gomod_update} true "${commit_msg_tag}" "${recursive_delete_pattern}"
            fi
            dst_needs_gomod_update=false
            dst_merge_point_commit=$(git rev-parse HEAD)
        fi
        k_pending_merge_commit="${k_new_pending_merge_commit}"

        # stop the loop?
        if [ ${f_mainline_commit} = FLUSH_PENDING_MERGE_COMMIT ]; then
            break
        fi

        # is it a merge or a single commit on the mainline to apply?
        if [ ${dst_branch} != master ] && is-merge-with-master ${k_mainline_commit}; then
            echo "Deferring master merge commit ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
        elif [ ${dst_branch} != master ] && [ -n "${k_pending_merge_commit}" ] && is-merge-with-master "${k_pending_merge_commit}"; then
            echo "Skipping master commit ${k_mainline_commit}: $(commit-subject ${f_mainline_commit}). Master merge commit ${k_pending_merge_commit} is pending."
        elif ! is-merge ${f_mainline_commit} || pick-merge-as-single-commit ${k_mainline_commit}; then
            local pick_args=""
            if is-merge ${f_mainline_commit}; then
                pick_args="-m 1"
                echo "Cherry-picking k8s.io/kubernetes merge-commit  ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
            else
                echo "Cherry-picking k8s.io/kubernetes single-commit ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
            fi

            # reset go.mod?
            local squash_commits=1
            if gomod-changes ${f_mainline_commit}; then
                reset-gomod ${f_mainline_commit}^
                squash_commits=$[${squash_commits} + 1] # squash the cherry-pick into the go.mod reset commit below
                dst_needs_gomod_update=true
            fi

            # finally cherry-pick
            if ! GIT_COMMITTER_DATE="$(commit-date ${f_mainline_commit})" git cherry-pick --keep-redundant-commits ${pick_args} ${f_mainline_commit} >/dev/null; then
                echo
                show-working-dir-status
                return 1
            fi

            # potentially squash go.mod reset commit
            squash ${squash_commits}

            # if there is no pending merge commit, update go.mod because this could be a target of a tag
            if ! skip-gomod-update ${k_mainline_commit} && [ -z "${k_pending_merge_commit}" ]; then
                fix-gomod "${deps}" "${required_packages}" "${base_package}" "${is_library}" ${dst_needs_gomod_update} true ${commit_msg_tag} "${recursive_delete_pattern}"
            fi
            dst_needs_gomod_update=false
            dst_merge_point_commit=$(git rev-parse HEAD)
        else
            # find **latest** (in the sense of least distance to ${f_mainline_commit}) common ancestor of both parents
            # of ${f_mainline_commit}. If the PR had no merge commits, this is the actual fork point. If there have been
            # merges, we will get the latest of those merges. Everything between that and ${f_mainline_commit} will be
            # linear. This will potentially drop commit history, but no actual changes.
            #
            # The simple merge case:                       The interleaved merge case:
            #
            #   M─┐ f_mainline_commit                        M───┐ f_mainline_commit
            #   │ o f_mainline_commit^2                      │   o f_mainline_commit^2
            #   o │ f_mainline_commit^1                      o   │ f_mainline_commit^1
            #   o │                                          M─┐ │
            #   │─M f_latest_merge_commit                    │ o │
            #   o │ f_latest_branch_point_commit             │ │─M f_latest_merge_commit
            #   │─M <--- some older merge                    │ o │ f_latest_branch_point_commit
            #   │ o <--- lost from the history               │─┘ o <--- lost from the history
            #   │─┘                                          │───M <--- some older merge
            #   │                                            o   │
            #                                                │───┘
            #                                                │
            local f_latest_branch_point_commit=$(git merge-base --octopus ${f_mainline_commit}^1 ${f_mainline_commit}^2)
            if [ -z "${f_latest_branch_point_commit}" ]; then
                echo "No branch point found for PR merged through ${f_mainline_commit}. Odd."
                return 1
            fi

            # start cherry-picking with latest merge, squash everything before.
            # Note: we want to ban merge commits on feature branches, compare https://github.com/kubernetes/kubernetes/pull/51176.
            #       Until that or something equivalent is in place, we do best effort here not to fall over those merge commits.
            local f_first_pick_base=${f_latest_branch_point_commit}
            local f_latest_merge_commit=$(git log --merges --format='%H' --ancestry-path -1 ${f_latest_branch_point_commit}..${f_mainline_commit}^2)
            if [ -n "${f_latest_merge_commit}" ]; then
                echo "Cherry-picking squashed k8s.io/kubernetes branch-commits $(kube-commit ${commit_msg_tag} ${f_latest_branch_point_commit})..$(kube-commit ${commit_msg_tag} ${f_latest_merge_commit}) because the last one is a merge: $(commit-subject ${f_latest_merge_commit})"

                # reset go.mod?
                local squash_commits=1
                if gomod-changes ${f_latest_branch_point_commit} ${f_latest_merge_commit}; then
                    reset-gomod ${f_latest_branch_point_commit}
                    squash_commits=$[${squash_commits} + 1] # squash the cherry-pick into the go.mod reset commit below
                    dst_needs_gomod_update=true
                fi

                if ! git diff --quiet --exit-code ${f_latest_branch_point_commit} ${f_latest_merge_commit}; then
                    if ! git diff ${f_latest_branch_point_commit} ${f_latest_merge_commit}  | git apply --index; then
                        echo
                        show-working-dir-status
                        return 1
                    fi
                fi
                git commit --allow-empty -q -m "sync: squashed up to merge $(kube-commit ${commit_msg_tag} ${f_latest_merge_commit}) in ${k_mainline_commit}" --date "$(commit-date ${f_latest_merge_commit})" --author "$(commit-author ${f_latest_merge_commit})"
                ensure-clean-working-dir

                # potentially squash go.mod reset commit
                squash ${squash_commits}

                # we start cherry-picking now from f_latest_merge_commit up to the actual Github merge into the mainline
                f_first_pick_base=${f_latest_merge_commit}
            fi
            for f_commit in $(git log --format='%H' --reverse ${f_first_pick_base}..${f_mainline_commit}^2); do
                # reset go.mod?
                local squash_commits=1
                if gomod-changes ${f_commit}; then
                    reset-gomod $(state-before-commit ${f_commit})
                    squash_commits=$[${squash_commits} + 1] # squash the cherry-pick into the go.mod reset commit below
                    dst_needs_gomod_update=true
                fi

                echo "Cherry-picking k8s.io/kubernetes branch-commit $(kube-commit ${commit_msg_tag} ${f_commit}): $(commit-subject ${f_commit})."
                if ! GIT_COMMITTER_DATE="$(commit-date ${f_commit})" git cherry-pick --keep-redundant-commits ${f_commit} >/dev/null; then
                    echo
                    show-working-dir-status
                    return 1
                fi
                ensure-clean-working-dir

                # potentially squash go.mod reset commit
                squash ${squash_commits}
            done

            # commit empty PR merge. This will carry the actual SHA1 from the upstream commit. It will match tags as well.
            echo "Cherry-picking k8s.io/kubernetes branch-merge  ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
            local date=$(commit-date ${f_mainline_commit}) # author and committer date is equal for PR merges
            git reset -q $(GIT_COMMITTER_DATE="${date}" GIT_AUTHOR_DATE="${date}" git commit-tree -p ${dst_merge_point_commit} -p HEAD -m "$(commit-message ${f_mainline_commit})" HEAD^{tree})

            # reset to mainline state which is guaranteed to be correct.
            # On the feature branch we might have reset to an too early state:
            #
            # In k8s.io/kubernetes:              Linearized in published repo:
            #
            #   M───┐ f_mainline_commit, result B  M─┐ result B
            #   M─┐ │ result A                     │ o change B
            #   │ o │ change A                     │─┘
            #   │ │ o change B                     M─┐ result A
            #   │─┘ │ base A                       │ o change A
            #   │───┘ base B                       │─┘
            #
            # Compare that with amending f_mainline_commit's go.mod into the HEAD,
            # we get result B in the linearized version as well. In contrast with this,
            # we would end up with "base B + change B" which misses the change A changes.
            amend-gomod-at ${f_mainline_commit}

            if ! skip-gomod-update ${k_mainline_commit}; then
                fix-gomod "${deps}" "${required_packages}" "${base_package}" "${is_library}" ${dst_needs_gomod_update} true ${commit_msg_tag} "${recursive_delete_pattern}"
            fi
            dst_needs_gomod_update=false
            dst_merge_point_commit=$(git rev-parse HEAD)
        fi

        ensure-clean-working-dir
    done

    # get consistent and complete go.mod on each sync. Skip if nothing changed.
    # NOTE: we cannot skip collapsed-kube-commit-mapper below because its
    #       output depends on upstream's HEAD.
    echo "Fixing up go.mod after a complete sync"
    if [ $(git rev-parse HEAD) != "${dst_old_head}" ] || [ "${new_branch}" = "true" ]; then
        fix-gomod "${deps}" "${required_packages}" "${base_package}" "${is_library}" true true ${commit_msg_tag} "${recursive_delete_pattern}"
    else
        # update go.mod without squashing because it would mutate a published commit
        fix-gomod "${deps}" "${required_packages}" "${base_package}" "${is_library}" true false ${commit_msg_tag} "${recursive_delete_pattern}"
    fi

    # create look-up file for collapsed upstream commits
    local repo=$(basename ${PWD})
    echo "Writing k8s.io/kubernetes commit lookup table to ../kube-commits-${repo}-${dst_branch}"
    /collapsed-kube-commit-mapper --commit-message-tag $(echo ${source_repo_name} | sed 's/^./\L\u&/')-commit --source-branch refs/heads/upstream-branch > ../kube-commits-${repo}-${dst_branch}
}

# for some PR branches cherry-picks fail. Put commits here where we only pick the whole merge as a single commit.
function pick-merge-as-single-commit() {
    grep -F -q -x "$1" <<EOF
25ebf875b4235cb8f43be2aec699d62e78339cec
8014d73345233c773891f26008e55dc3b5232c7c
536cee71b4dcb74fa7c80fdd6a709cdbf970e4a2
69bd30507559be3dea905686b46bc3295c951f45
64718f678695884c93d6d3df8f5799614746bea2
bc53b97ceb25338570a853845c4cdd295468ed61
EOF
}

# if a PR added incorrect go.mod changes (eg: client-go depending on apiserver), go.mod update will fail.
# so we skip go.mod generation for these commits.
function skip-gomod-update() {
    grep -F -q -x "$1" <<EOF
e2a017327c1af628f4f0069cbd49865ad1e81975
fd0df59f5ba786cb25329e3a9d2793ad4227ed87
EOF
}

# TODO(nikhita): remove this when release tooling satisifies the k/k
# mainline invariant again. See https://github.com/kubernetes/release/issues/2337.
# TODO(nikhita): add link to publishing-bot issue with more details.
# Due to some changes in the release tooling, it is now possible to have
# two k/k PRs/commits with the same k/k mainline commit. This causes one of the
# k/k commit to not be published.
# We manually published this missing commit in the staging repo.
# Due to this, the bot now re-publishes commits created after the manually added commit.
# If a new branch exists at one of the re-published commits, it leads to
# two dst_branch_point_commits.
# The list of k/k commits here ensures that only a single dst_branch_point_commit
# is picked for them.
function pick-single-dst-branch-point-commit() {
    grep -F -q -x "$1" <<EOF
f1d5d4df5a786a3387e1f39f59941f2c6fb4299d
e4952f32b79b69bfa9333ff9da26a2da13859148
EOF
}

# amend-gomod-at checks out the go.mod at the given commit and amend it to the previous commit.
function amend-gomod-at() {
    if [ -f go.mod ]; then
        git checkout ${f_mainline_commit} go.mod go.sum # reset to mainline state which is guaranteed to be correct
        git commit --amend --no-edit -q
    fi
}

function commit-date() {
    git show --format="%aD" -q ${1}
}

function committer-date() {
    git show --format="%cD" -q ${1}
}

function commit-author() {
    git show --format="%an <%ae>" -q ${1}
}

function short-commit-message() {
    git show --format=short -q ${1}
}

function commit-message() {
    git show --format="%B" -q ${1}
}

function commit-subject() {
    git show --format="%s" -q ${1}
}

# rewrites git history to *only* include $subdirectory
function filter-branch() {
    local commit_msg_tag="${1}"
    local subdirectory="${2}"
    local recursive_delete_pattern="${3}"
    echo "Running git filter-branch ..."
    local index_filter=""
    if [ -n "${recursive_delete_pattern}" ]; then
        local patterns=()
        local p=""
        index_filter="git rm -q --cached --ignore-unmatch -r"
        IFS=" " read -ra patterns <<<"${recursive_delete_pattern}"
        for p in "${patterns[@]}"; do
            index_filter+=" '${p}'"
        done
    fi
    git filter-branch -f --index-filter "${index_filter}" --msg-filter 'awk 1 && echo && echo "'"${commit_msg_tag}"': ${GIT_COMMIT}"' --subdirectory-filter "${subdirectory}" -- ${4} ${5} >/dev/null
}

function is-merge() {
    if ! grep -q "^Merge: " <<<"$(short-commit-message ${1})"; then
        return 1
    fi
}

function is-merge-with-master() {
    if ! grep -q "^Merge remote-tracking branch 'origin/master'" <<<"$(short-commit-message ${1})"; then
        return 1
    fi
}

function ensure-clean-working-dir() {
    if ! git diff HEAD --exit-code &>/dev/null; then
        echo "Expected clean git working dir. It's not:"
        show-working-dir-status
        return 1
    fi
}

function show-working-dir-status() {
    git diff -a --cc | sed 's/^/    /'
    echo
    git status | sed 's/^/    /'
}

function gomod-changes() {
    if [ -n "${2:-}" ]; then
        ! git diff --exit-code --quiet ${1} ${2} -- go.mod go.sum
    else
        ! git diff --exit-code --quiet $(state-before-commit ${1}) ${1} -- go.mod go.sum
    fi
}

function state-before-commit() {
    if git rev-parse --verify ${1}^1 &>/dev/null; then
        echo ${1}^
    else
        printf '' | git hash-object -t tree --stdin
    fi
}

function branch-commit() {
    local commit_msg_tag="${1}"
    local log_args="${4:-""}"

    git log --grep "${commit_msg_tag}: ${2}" --format='%H' ${log_args} ${3:-HEAD} 
}

function last-kube-commit() {
    local commit_msg_tag="${1}"
    git log --format="%B" ${2:-HEAD} | grep "^${commit_msg_tag}: " | head -n 1 | sed "s/^${commit_msg_tag}: //g"
}

function kube-commit() {
    local commit_msg_tag="${1}"
    commit-message ${2:-HEAD} | grep "^${commit_msg_tag}: " | sed "s/^${commit_msg_tag}: //g"
}

# find the rev when the given commit was merged into the branch
function git-find-merge() {
    # taken from https://stackoverflow.com/a/38941227: intersection of both files, with the order of the second
    awk 'NR==FNR{a[$1]++;next} a[$1] ' \
        <(git rev-list ${1}^1..${2:-master} --first-parent) \
        <(git rev-list ${1}..${2:-master} --ancestry-path; git rev-parse ${1}) \
    | tail -1
}

# find the first common commit on the first-parent mainline of two branches, i.e. the point where a fork was started.
# By considering only the mainline of both branches, this will handle merges between the two branches by skipping
# them in the search.
function git-fork-point() {
    # taken from https://stackoverflow.com/a/38941227: intersection of both files, with the order of the second
    awk 'NR==FNR{a[$1]++;next} a[$1] ' \
        <(git rev-list ${2:-master} --first-parent) \
        <(git rev-list ${1:-HEAD} --first-parent) \
    | head -1
}

function git-index-clean() {
    if git diff --cached --exit-code &>/dev/null; then
        return 0
    fi
    return 1
}

function apply-recursive-delete-pattern() {
    local recursive_delete_pattern="${1}"
    if [ -z "${recursive_delete_pattern}" ]; then
        return
    fi

    local split_recursive_delete_pattern
    read -r -a split_recursive_delete_pattern <<< "${recursive_delete_pattern}"
    git rm -q --ignore-unmatch -r "${split_recursive_delete_pattern[@]}"
    git add -u
    if ! git-index-clean; then
        echo "Deleting files recursively: ${recursive_delete_pattern}"
        git commit -m "sync: initially remove files ${recursive_delete_pattern}"
    fi
}

function fix-gomod() {
    if [ "${PUBLISHER_BOT_SKIP_GOMOD:-}" = true ]; then
        return 0
    fi

    local deps="${1}"
    local required_packages="${2}"
    local base_package="${3}"
    local is_library="${4}"
    local needs_gomod_update="${5}"
    local squash="${6:-true}"
    local commit_msg_tag="${7}"
    local recursive_delete_pattern="${8}"

    local dst_old_commit=$(git rev-parse HEAD)
    if [ -f go.mod ]; then
        checkout-deps-to-kube-commit "${commit_msg_tag}" "${deps}" "${base_package}"
        update-deps-in-gomod "${deps}" "${base_package}"
    fi

    # squash go.mod commits, either into ${dst_old_commit} or into _one_ new commit
    if git diff --exit-code ${dst_old_commit} &>/dev/null; then
        echo "Remove redundant go.mod commits on-top of ${dst_old_commit}."
        git reset --soft -q ${dst_old_commit}
    elif [ "${squash}" = true ]; then
        echo "Amending last merge with go.mod changes."
        git reset --soft -q ${dst_old_commit}
        git commit -q --amend --allow-empty -C ${dst_old_commit}
    else
        echo "Squashing go.mod commits into one."
        local old_head="$(git rev-parse HEAD)"
        git reset --soft -q ${dst_old_commit}
        git commit -q --allow-empty -m "sync: update go.mod"
    fi

    ensure-clean-working-dir
}

# Reset go.mod to what it looked like in the given commit $1. Always create a
# commit, even an empty one.
function reset-gomod() {
    local f_clean_commit=${1}

    # checkout or delete go.mod
    if [ -n "$(git ls-tree ${f_clean_commit}^{tree} go.mod)" ]; then
        git checkout ${f_clean_commit} go.mod go.sum
        git add go.mod go.sum
    elif [ -f go.mod ]; then
        rm -f go.mod go.sum
        git rm -f go.mod go.sum
    fi

    # commit go.mod unconditionally
    git commit -q -m "sync: reset go.mod" --allow-empty
}

# Squash the last $1 commits into one, with the commit message of the last.
function squash() {
    local head=$(git rev-parse HEAD)
    git reset -q --soft HEAD~${1:-2}
    GIT_COMMITTER_DATE=$(committer-date ${head}) git commit --allow-empty -q -C ${head}
}

# update-deps-in-gomod updates go.mod according to checked out versions of dependencies.
#
# "deps" lists the dependent k8s.io/* repos and branches. For example, if the
# function is handling the release-1.6 branch of k8s.io/apiserver, deps is
# expected to be "apimachinery:release-1.6,client-go:release-3.0". Dependencies
# are expected to be separated by ",", and the name of the dependent repo and
# the branch name are expected to be separated by ":".
#
# This function assumes to be called at the root of the repository that's going to be published.
# This function assumes the branch that need update is checked out, for the current repo and for dependencies.
# This function assumes it's the last step in the publishing process that's going to generate commits.
update-deps-in-gomod() {
    if [ ! -f go.mod ]; then
        return 0
    fi

    local deps_array=()
    IFS=',' read -a deps_array <<< "${1}"
    local dep_count=${#deps_array[@]}
    local base_package=${2}

    # if dependencies exist, dep_packages is a comma separated list of {base_package}/{dep}. Eg: "k8s.io/api,k8s.io/apimachinery"
    local dep_packages=""
    if [ "$dep_count" != 0 ]; then
      dep_packages="$(echo ${1} | tr "," "\n" | sed -e 's/:.*//' -e s,^,"${base_package}/", | paste -sd "," -)"
    fi

    for (( i=0; i<${dep_count}; i++ )); do
        local dep="${deps_array[i]%%:*}"
        local dep_commit=$(cd ../${dep}; gomod-pseudo-version)
        echo "Updating ${base_package}/${dep} to point to ${dep_commit}"
        GO111MODULE=on go mod edit -fmt -require "${base_package}/${dep}@${dep_commit}"
        GO111MODULE=on go mod edit -fmt -replace "${base_package}/${dep}=${base_package}/${dep}@${dep_commit}"
    done

    GO111MODULE=on go mod edit -json | jq -r '.Replace[]? | select(.New.Path | startswith("../")) | "-dropreplace \(.Old.Path)"' | GO111MODULE=on xargs -L 100 go mod edit -fmt
    
    # TODO(nikhita): remove this after go.sum values are fixed.
    #
    # gomod-zip copied go's zip creation code but they had diverged.
    # due to this, gomod-zip created different zip files in the cache
    # compared to what go mod download would create.
    #
    # From Go 1.15.11 and Go 1.16.3, go automatically derives the ziphash
    # from the zip file in the cache - https://github.com/golang/go/issues/44812.
    # This meant that go added incorrect hash values to go.sum because these
    # were derived from the zip files produced by the diverged gomod-zip code.
    #
    # So remove go.sum here and regenerate again using
    # go mod download and go mod tidy.
    [ -s go.sum ] && rm go.sum

    GO111MODULE=on GOPRIVATE="${dep_packages}" GOPROXY=https://proxy.golang.org go mod download
    fixAmbiguousImports
    GOPROXY="file://${GOPATH}/pkg/mod/cache/download,https://proxy.golang.org" GO111MODULE=on GOPRIVATE="${dep_packages}" go mod tidy

    git add go.mod go.sum

    # double check that we got all dependencies
    if grep 000000000000 go.sum; then
        echo "Invalid go.mod created. Failing."
        exit 1
    fi

    # check if there are new contents
    if git-index-clean; then
        echo "go.mod hasn't changed!"
    else
        echo "Committing go.mod"
        git commit -q -m "sync: update go.mod"
    fi

    # nothing should be left
    ensure-clean-working-dir
}

function fixAmbiguousImports() {
   # ref: https://github.com/kubernetes/publishing-bot/issues/304
   # TODO(nikhita): remove after https://github.com/kubernetes/kubernetes/pull/114829 gets published.
   if [ -n "$(go list -m cloud.google.com/go)" ]; then
    go get "cloud.google.com/go@$(go list -m -json cloud.google.com/go | jq -r '.Version')"
   fi
}

gomod-pseudo-version() {
    TZ=GMT git show -q --pretty='format:v0.0.0-%cd-%h' --date='format-local:%Y%m%d%H%M%S' --abbrev=12
}

# checkout the dependencies to the versions corresponding to the kube commit of HEAD
checkout-deps-to-kube-commit() {
    local commit_msg_tag="${1}"
    local deps=()
    IFS=',' read -a deps <<< "${2}"
    local base_package=${3}
    local dep_count=${#deps[@]}

    # if dependencies exist, dep_packages is a comma separated list of {base_package}/{dep}. Eg: "k8s.io/api,k8s.io/apimachinery"
    local dep_packages=""
    if [ "$dep_count" != 0 ]; then
      dep_packages="$(echo ${2} | tr "," "\n" | sed -e 's/:.*//' -e s,^,"${base_package}/", | paste -sd "," -)"
    fi

    # get last k8s.io/kubernetes commit on HEAD ...
    local k_last_kube_commit="$(last-kube-commit ${commit_msg_tag} HEAD)"
    if [ -z "${k_last_kube_commit}" ]; then
        echo "No k8s.io/kubernetes commit found in the history of HEAD."
        return 1
    fi

    # ... and get possible merge point of it (in case of dropped fast-forward merges this
    # might have been dropped on HEAD).
    local k_last_kube_merge=$(git-find-merge "${k_last_kube_commit}" upstream-branch)

    for (( i=0; i<${dep_count}; i++ )); do
        local dep="${deps[i]%%:*}"
        local branch="${deps[i]##*:}"

        echo "Looking up which commit in the ${branch} branch of k8s.io/${dep} corresponds to k8s.io/kubernetes commit ${k_last_kube_merge}."
        local k_commit=""
        local dep_commit=""
        read k_commit dep_commit <<<$(look -b ${k_last_kube_merge} ../kube-commits-${dep}-${branch})
        if [ -z "${dep_commit}" ]; then
            echo "Could not find corresponding k8s.io/${dep} commit for kube commit ${k_last_kube_commit}."
            return 1
        fi

        pushd ../${dep} >/dev/null
            echo "Checking out k8s.io/${dep} to ${dep_commit}"
            git checkout -q "${dep_commit}"

            local pseudo_version=$(gomod-pseudo-version)
            local cache_dir="${GOPATH}/pkg/mod/cache/download/${base_package}/${dep}/@v"
            if [ -f "${cache_dir}/list" ] && grep -q "${pseudo_version}" "${cache_dir}/list"; then
            	echo "Pseudo version ${pseudo_version} is already packaged up."
            else
            	echo "Packaging up pseudo version ${pseudo_version} into go mod cache..."
            	mkdir -p "${cache_dir}"
            	cp go.mod "${cache_dir}/${pseudo_version}.mod"
                echo "{\"Version\":\"${pseudo_version}\",\"Name\":\"$(git rev-parse HEAD)\",\"Short\":\"$(git show -q --abbrev=12 --pretty='format:%h' HEAD)\",\"Time\":\"$(TZ=GMT git show -q --pretty='format:%cd' --date='format-local:%Y-%m-%dT%H:%M:%SZ')\"}" > "${cache_dir}/${pseudo_version}.info"
                pushd "${GOPATH}/src" >/dev/null
                /gomod-zip --package-name="${base_package}/${dep}" --pseudo-version="${pseudo_version}"
                popd >/dev/null
                echo "${pseudo_version}" >> "${cache_dir}/list"
            fi
        popd >/dev/null
    done
}

