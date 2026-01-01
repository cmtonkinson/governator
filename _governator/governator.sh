#!/usr/bin/env bash
set -euo pipefail

# pseudocode algorithm for the main loop:
# 
# governator.lock exists?
#   yes: emit a warning and exit
# create file governator.lock
#
# local git changes?
#   yes: emit a warning and exit
#
# checkout main
# pull origin main
#
# for each origin/worker/* branch:
#   skip if branch is in .governator/failed-merges.log
#   process(branch)
#   remove worker tmp dir
#
# process(branch):
#   fetch origin
#   checkout the branch
#   task = find the task file from the branch name
#   switch task:
#     case "task-worked":
#       decision = code_review(branch)
#       annotate task file with comments
#       remove code review tmp dir
#       switch decision
#         case "approve": move task to task-done
#         case "reject":  move task to task-assigned
#         else:           move task to task-blocked
#     case "task-feedback":
#       provide feedback in task
#       move task to task-assigned
#     else:
#       emit warning
#       move task to task-blocked
#   commit
#   checkout main
#   will branch do a clean ff merge into main?
#     no:
#       emit error
#       annotate task file with comments/errors/etc.
#       move task to task-blocked
#       emit branch name and timestamp into .governator/failed-merges.log
#   delete the local worker branch
#   delete the remote worker branch
#   push main
#
#
# assign_pending_tasks():
#   get list of task files in task-backlog/
#   for each task file:
#     skip if task is mentioned in .governator/in-flight.log
#     skip if worker is mentioned in .governator/in-flight.log
#     assign(task_file, worker)
#     emit "<task> -> <worker>" into .governator/in-flight.log
#
# assign():
#   checkout main
#   move task file
#   annotate
#   commit
#   push main
#
#   clone origin into /tmp/governator-<project_name>-<worker_name>-<task_name>-<timestamp>
#   cd there
#   checkout new branch from main worker/<worker_name>/<task_name>
#   is expertise defined?
#     yes:
#       get file contents
#       remove all comment lines and empty lines
#       convert the one-entry-per-line list into a single string with each entry ", "-joined
#       write a new file named is_expert.md with the content "You have deep expertise in <joined-string>."
#   invoke codex non-interactively and non-blockingly
#     with instructions "Read and follow the instructions in the following files, in this order: worker_contract.md, worker-roles/<worker_name>.md, (is_expert.md if exists) and task-assigned/<task>.md"
#
# # Note the reviewer instructions will include leaving explicit approve/reject/block content in review.json
# code_review(branch):
#   clone origin into /tmp/governator-<project_name>-reviewer-<task_name>-<timestamp>
#   cd there
#   checkout branch
#   is expertise defined?
#     yes:
#       get file contents
#       remove all comment lines and empty lines
#       convert the one-entry-per-line list into a single string with each entry ", "-joined
#       write a new file named is_expert.md with the content "You have deep expertise in <joined-string>."
#   invoke codex non-interactively and block until it exits
#     with instructions "Read and follow the instructions in the following files, in this order: special-roles/reviewer.md (and is_expert.md if exists). The task given was <path to task file>.
#   return contents of review.json
